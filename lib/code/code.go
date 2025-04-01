package code

import (
	"github.com/klauspost/reedsolomon"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
)

var _ Codec = (*DataCoder)(nil)

type DataCoder struct {
	*segment.Prefix
	blsKey       pdpcommon.KeySet
	dv           pdpcommon.DataVerifier
	chunkCount   int
	prefixSize   int
	tagCount     int
	tagSize      int
	fragmentSize int // prefixSize + segsize + tagCount*tagsize
}

// NewDataCoder 构建一个dataformat配置
func NewDataCoder(keyset pdpcommon.KeySet, version, policy, dataCount, parityCount, tagFlag, segSize int) (*DataCoder, error) {
	if segSize != DefaultSegSize {
		segSize = DefaultSegSize
	}

	switch policy {
	case RsPolicy:
	case MulPolicy:
		parityCount = dataCount + parityCount - 1
		dataCount = 1
	default:
		return nil, xerrors.Errorf("policy %d is not supported", policy)
	}

	pre := &segment.Prefix{
		Version:     uint32(version),
		Policy:      uint32(policy),
		DataCount:   uint32(dataCount),
		ParityCount: uint32(parityCount),
		TagFlag:     uint32(tagFlag),
		SegSize:     uint32(segSize),
	}

	return NewDataCoderWithPrefix(keyset, pre)
}

// NewDataCoderWithDefault creates a new datacoder with default
func NewDataCoderWithDefault(keyset pdpcommon.KeySet, policy, dataCount, pairtyCount int, userID, fsID string) (*DataCoder, error) {
	return NewDataCoder(keyset, CurrentVersion, policy, dataCount, pairtyCount, DefaultTagFlag, DefaultSegSize)
}

// NewDataCoderWithBopts contructs a new datacoder with bucketops
func NewDataCoderWithBopts(keyset pdpcommon.KeySet, bo *pb.BucketOption) (*DataCoder, error) {
	return NewDataCoder(keyset, int(bo.Version), int(bo.Policy), int(bo.DataCount), int(bo.ParityCount), int(bo.TagFlag), int(bo.SegSize))
}

// NewDataCoderWithPrefix creates a new datacoder with prefix
func NewDataCoderWithPrefix(keyset pdpcommon.KeySet, p *segment.Prefix) (*DataCoder, error) {
	vkey, err := pdp.NewDataVerifier(keyset.PublicKey(), keyset.SecreteKey())
	if err != nil {
		return nil, err
	}

	d := &DataCoder{
		Prefix: p,
		blsKey: keyset,
		dv:     vkey,
	}
	err = d.preCompute()
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DataCoder) preCompute() error {
	d.prefixSize = DefaultPrefixLen
	dc := int(d.DataCount)
	pc := int(d.ParityCount)
	if dc < 1 || pc < 1 {
		return xerrors.Errorf("policy is not supported")
	}

	d.chunkCount = dc + pc
	d.tagCount = 2 + (pc-1)/dc

	s, ok := pdpcommon.TagMap[int(d.TagFlag)]
	if !ok {
		s = 48
	}

	d.tagSize = int(s)

	d.fragmentSize = d.prefixSize + int(d.SegSize) + d.tagSize*d.tagCount
	return nil
}

// ncidPrefix为  fsID_bucketID_stripeID
func (d *DataCoder) Encode(ncidPrefix segment.SegmentID, data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, xerrors.New("data length is wrong")
	}

	dc := int(d.DataCount)
	pc := int(d.ParityCount)

	if len(data) > dc*int(d.SegSize) {
		return nil, xerrors.New("data length is too large")
	}

	// 如果长度不够则填充
	if len(data) < dc*int(d.SegSize) {
		temp := make([]byte, dc*int(d.SegSize)-len(data))
		data = append(data, temp...)
	}

	preData := d.Prefix.Serialize()

	offset := d.prefixSize

	stripe := make([][]byte, d.chunkCount)
	dataGroup := make([][]byte, d.chunkCount)
	tagGroup := make([][]byte, d.chunkCount*d.tagCount)

	for i := 0; i < d.chunkCount; i++ {
		stripe[i] = make([]byte, d.fragmentSize)
		copy(stripe[i][:offset], preData)
		if i < dc {
			copy(stripe[i][offset:offset+int(d.SegSize)], data[i*int(d.SegSize):(i+1)*int(d.SegSize)])
		}

		dataGroup[i] = stripe[i][offset : offset+int(d.SegSize)]
	}

	enc, err := reedsolomon.New(int(dc), int(pc))
	if err != nil {
		return nil, err
	}

	encP, err := reedsolomon.New(d.chunkCount, d.chunkCount*(d.tagCount-1))
	if err != nil {
		return nil, err
	}

	switch d.Prefix.Policy {
	case MulPolicy:
		for j := dc; j < d.chunkCount; j++ {
			copy(dataGroup[j], dataGroup[0])
		}
	case RsPolicy:
		err = enc.Encode(dataGroup)
		if err != nil {
			return nil, err
		}
	default:
		return nil, xerrors.Errorf("policy %d is not supported", d.Prefix.Policy)
	}

	offset += int(d.SegSize)

	for j := 0; j < d.chunkCount; j++ {
		ncidPrefix.SetChunkID(uint32(j))

		tag, err := d.GenTag(ncidPrefix.Bytes(), dataGroup[j])
		if err != nil {
			return nil, err
		}
		copy(stripe[j][offset:offset+d.tagSize], tag)
	}

	for j := 0; j < d.tagCount; j++ {
		for k := 0; k < d.chunkCount; k++ {
			tagGroup[j*d.chunkCount+k] = stripe[k][offset+j*d.tagSize : offset+(j+1)*d.tagSize]
		}
	}

	err = encP.Encode(tagGroup)
	if err != nil {
		return nil, err
	}

	return stripe, nil
}

// name is fsID_bucketID_stripeID; if set, verify tag;
func (d *DataCoder) Decode(name segment.SegmentID, stripe [][]byte) ([]byte, error) {
	ok, scount, err := d.VerifyStripe(name, stripe)
	if err != nil {
		return nil, err
	}

	if scount < int(d.DataCount) {
		return nil, xerrors.New("the recovered data is incorrect")
	}

	if !ok {
		err := d.Recover(nil, stripe)
		if err != nil {
			return nil, err
		}
	}

	res := make([]byte, int(d.DataCount)*int(d.SegSize))
	for j := 0; j < int(d.DataCount); j++ {
		copy(res[j*(int(d.SegSize)):], stripe[j][d.prefixSize:d.prefixSize+int(d.SegSize)])
	}
	return res, nil
}

// result: CanDirectRead, number of Good Chunk, canRecover
// if name != "", verify tag
func (d *DataCoder) VerifyStripe(name segment.SegmentID, stripe [][]byte) (bool, int, error) {
	if len(stripe) < int(d.DataCount) {
		return false, 0, xerrors.Errorf("stripe data is not enough")
	}

	good := 0
	directRead := true
	d.dv.Reset()
	for j := 0; j < len(stripe); j++ {
		if len(stripe[j]) != d.fragmentSize {
			stripe[j] = nil
		} else {
			// verify prefix
			pre, _, err := segment.DeserializePrefix(stripe[j])
			if err != nil || !d.VerifyPrefix(pre) {
				stripe[j] = nil
				continue
			}

			good++
			tag := stripe[j][d.prefixSize+int(d.SegSize) : d.prefixSize+int(d.SegSize)+d.tagSize]
			if name != nil {
				name.SetChunkID(uint32(j))
				seg := stripe[j][d.prefixSize : d.prefixSize+int(d.SegSize)]
				err := d.dv.Add(name.Bytes(), seg, tag)
				if err != nil {
					// tag is wrong
					stripe[j] = nil
					good--
				}
			} else {
				err := pdp.CheckTag(pdpcommon.PDPV2, tag)
				if err != nil {
					// tag is wrong
					stripe[j] = nil
					good--
				}
			}
		}
	}

	for j := 0; j < int(d.DataCount); j++ {
		if len(stripe[j]) == 0 {
			directRead = false
			break
		}
	}

	if good < int(d.DataCount) {
		return directRead, good, xerrors.New("the recovered data is incorrect")
	}

	if name != nil {
		ok, err := d.dv.Result()
		// verify each chunk
		if !ok || err != nil {
			good = 0
			for j := 0; j < len(stripe); j++ {
				//if good >= int(d.DataCount) {
				//	stripe[j] = nil
				//	continue
				//}
				if len(stripe[j]) == d.fragmentSize {
					name.SetChunkID(uint32(j))
					ok, err := d.VerifyChunk(name, stripe[j])
					if err != nil || !ok {
						stripe[j] = nil
						if j < int(d.DataCount) {
							directRead = false
						}
						continue
					}
					good++
				}
			}
		}
	}

	return directRead, good, nil
}

func (d *DataCoder) VerifyChunk(name segment.SegmentID, data []byte) (bool, error) {
	if len(data) < d.fragmentSize {
		return false, xerrors.Errorf("data length %d is shorter than %d", len(data), d.fragmentSize)
	}

	seg := data[d.prefixSize : d.prefixSize+int(d.SegSize)]
	tag := data[d.prefixSize+int(d.SegSize) : d.prefixSize+int(d.SegSize)+d.tagSize]

	return d.blsKey.VerifyTag(name.Bytes(), seg, tag)
}

func (d *DataCoder) Recover(name segment.SegmentID, stripe [][]byte) error {
	_, scount, err := d.VerifyStripe(name, stripe)
	if err != nil {
		return err
	}

	if scount < int(d.DataCount) {
		return xerrors.New("the recovered data is incorrect")
	}

	fieldStripe := make([][]byte, d.chunkCount)
	for i := 0; i < d.chunkCount; i++ {
		if i >= len(stripe) {
			stripe = append(stripe, nil)
		}
		if len(stripe[i]) != d.fragmentSize {
			stripe[i] = nil
			fieldStripe[i] = nil
		} else {
			fieldStripe[i] = stripe[i][d.prefixSize:d.fragmentSize]
		}
	}

	err = d.recoverField(name, fieldStripe)
	if err != nil {
		return err
	}

	preData := d.Prefix.Serialize()
	for i := 0; i < d.chunkCount; i++ {
		if stripe[i] == nil {
			stripe[i] = make([]byte, 0, d.fragmentSize)
			stripe[i] = append(stripe[i], preData...)
			stripe[i] = append(stripe[i], fieldStripe[i]...)
		}
	}

	return nil
}

func (d *DataCoder) recoverField(name segment.SegmentID, stripe [][]byte) error {
	var fault = make(map[int]struct{})
	dataGroup := make([][]byte, d.chunkCount)
	for i := 0; i < d.chunkCount; i++ {
		if stripe[i] != nil {
			dataGroup[i] = stripe[i][:d.SegSize]
		} else {
			dataGroup[i] = nil
			fault[i] = struct{}{}
		}
	}

	err := d.recoverData(dataGroup, int(d.DataCount), int(d.ParityCount))
	if err != nil {
		return err
	}

	tagGroup := make([][]byte, d.chunkCount*d.tagCount)
	for j := 0; j < d.tagCount; j++ {
		for i := 0; i < d.chunkCount; i++ {
			if stripe[i] != nil {
				tagGroup[i+j*d.chunkCount] = stripe[i][int(d.SegSize)+j*d.tagSize : int(d.SegSize)+(j+1)*d.tagSize]
			} else {
				tagGroup[i+j*d.chunkCount] = nil
			}
		}
	}

	err = d.recoverData(tagGroup, d.chunkCount, d.chunkCount*(d.tagCount-1))
	if err != nil {
		return err
	}

	if name != nil {
		d.dv.Reset()
		for i := range fault {
			stripe[i] = make([]byte, 0, d.fragmentSize-d.prefixSize)
			stripe[i] = append(stripe[i], dataGroup[i]...)

			name.SetChunkID(uint32(i))

			err := d.dv.Add(name.Bytes(), dataGroup[i], tagGroup[i])
			if err != nil {
				return err
			}
		}

		ok, err := d.dv.Result()
		if !ok || err != nil {
			return xerrors.New("the recovered data is incorrect")
		}
	} else {
		for i := range fault {
			err := pdp.CheckTag(pdpcommon.PDPV2, tagGroup[i])
			if err != nil {
				return err
			}
			stripe[i] = make([]byte, 0, d.fragmentSize-d.prefixSize)
			stripe[i] = append(stripe[i], dataGroup[i]...)
		}
	}

	for j := 0; j < d.chunkCount*d.tagCount; {
		for i := 0; i < d.chunkCount; i++ {
			_, ok := fault[i]
			if ok {
				stripe[i] = append(stripe[i], tagGroup[j]...)
			}
			j++
		}
	}

	return nil
}

func (d *DataCoder) recoverData(data [][]byte, dc, pc int) error {
	switch {
	case dc > 1:
		enc, err := reedsolomon.New(dc, pc)
		if err != nil {
			return err
		}
		ok, err := enc.Verify(data)
		if err == reedsolomon.ErrShardNoData || err == reedsolomon.ErrTooFewShards {
			return err
		}
		if !ok {
			err = enc.Reconstruct(data)
			if err != nil {
				return err
			}
			ok, err = enc.Verify(data)
			if err != nil {
				return err
			}

			if !ok {
				return xerrors.Errorf("data is broken")
			}
		}
		return nil
	case dc == 1:
		var i int
		for i = 0; i < d.chunkCount; i++ {
			if data[i] != nil {
				break
			}
		}

		if i == d.chunkCount {
			return xerrors.New("the recovered data is incorrect")
		}

		for j := 0; j < d.chunkCount; j++ {
			if data[j] == nil {
				data[j] = make([]byte, 0)
				data[j] = append(data[j], data[i]...)
			}
		}
		return nil
	default:
		return xerrors.Errorf("policy is not supported")
	}
}
