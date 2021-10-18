package dataformat

import (
	"strconv"
	"strings"

	"github.com/klauspost/reedsolomon"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	mpb "github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type Codec interface {
	// name is fsID_bucketID_stripeID
	Encode(name string, data []byte) ([][]byte, error)

	// name is fsID_bucketID_stripeID; if set, verify tag;
	// name can set to "" as fast mode
	Decode(name string, stripe [][]byte) ([]byte, error)
	Recover(name string, stripe [][]byte) error
	VerifyStripe(name string, stripe [][]byte) (bool, int, error)
	// name is fsID_bucketID_stripeID_chunkID
	VerifyChunk(name string, data []byte) (bool, error)
}

type DataCoder struct {
	*Prefix
	blsKey       pdpcommon.KeySet
	dv           pdpcommon.DataVerifier
	chunkCount   int
	prefixSize   int
	tagCount     int
	tagSize      int
	fragmentSize int // prefixSize + segsize + tagCount*tagsize
}

// NewDataCoder 构建一个dataformat配置
func NewDataCoder(keyset pdpcommon.KeySet, version, policy, dataCount, parityCount, tagFlag, segSize int) (Codec, error) {
	if segSize != DefaultSegSize {
		segSize = DefaultSegSize
	}

	switch policy {
	case RsPolicy:
	case MulPolicy:
		parityCount = dataCount + parityCount - 1
		dataCount = 1
	default:
		return nil, ErrWrongPolicy
	}

	pre := &Prefix{
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
func NewDataCoderWithDefault(keyset pdpcommon.KeySet, policy, dataCount, pairtyCount int, userID, fsID string) (Codec, error) {
	return NewDataCoder(keyset, CurrentVersion, policy, dataCount, pairtyCount, DefaultTagFlag, DefaultSegSize)
}

// NewDataCoderWithBopts contructs a new datacoder with bucketops
func NewDataCoderWithBopts(keyset pdpcommon.KeySet, bo *mpb.BucketOption) (Codec, error) {
	return NewDataCoder(keyset, int(bo.Policy), int(bo.DataCount), int(bo.ParityCount), int(bo.Version), int(bo.TagFlag), int(bo.SegSize))
}

// NewDataCoderWithPrefix creates a new datacoder with prefix
func NewDataCoderWithPrefix(keyset pdpcommon.KeySet, p *Prefix) (Codec, error) {
	vkey := pdpv2.NewDataVerifier(keyset.PublicKey(), keyset.SecreteKey())
	if vkey == nil {
		return nil, ErrWrongCoder
	}

	d := &DataCoder{
		Prefix: p,
		blsKey: keyset,
		dv:     vkey,
	}
	err := d.preCompute()
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
		return ErrWrongPolicy
	}

	d.chunkCount = dc + pc
	d.tagCount = 2 + (pc-1)/dc

	s, ok := pdp.TagMap[int(d.TagFlag)]
	if !ok {
		s = 48
	}

	d.tagSize = int(s)

	d.fragmentSize = d.prefixSize + int(d.SegSize) + d.tagSize*d.tagCount
	return nil
}

// ncidPrefix为  fsID_bucketID_stripeID
func (d *DataCoder) Encode(ncidPrefix string, data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ErrDataLength
	}

	dc := int(d.DataCount)
	pc := int(d.ParityCount)

	if len(data) != dc*int(d.SegSize) {
		return nil, ErrDataLength
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
		return nil, ErrWrongPolicy
	}

	offset += int(d.SegSize)
	var res strings.Builder
	for j := 0; j < d.chunkCount; j++ {
		res.Reset()
		res.WriteString(ncidPrefix)
		res.WriteString(types.SegDelimiter)
		res.WriteString(strconv.Itoa(j))

		tag, err := d.GenTag([]byte(res.String()), dataGroup[j])
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

func (d *DataCoder) Decode(name string, stripe [][]byte) ([]byte, error) {
	ok, scount, err := d.VerifyStripe(name, stripe)
	if err != nil {
		return nil, err
	}

	if scount < int(d.DataCount) {
		return nil, ErrRecoverData
	}

	if !ok {
		err := d.Recover("", stripe)
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
func (d *DataCoder) VerifyStripe(name string, stripe [][]byte) (bool, int, error) {
	if len(stripe) < int(d.DataCount) {
		return false, 0, ErrDataBroken
	}

	good := 0
	directRead := true
	d.dv.Reset()
	var res strings.Builder
	for j := 0; j < len(stripe); j++ {
		if len(stripe[j]) != d.fragmentSize {
			stripe[j] = nil
		} else {
			// verify prefix
			pre, _, err := DeserializePrefix(stripe[j])
			if err != nil || !d.VerifyPrefix(pre) {
				stripe[j] = nil
				continue
			}

			good++
			tag := stripe[j][d.prefixSize+int(d.SegSize) : d.prefixSize+int(d.SegSize)+d.tagSize]
			if name != "" {
				res.Reset()
				res.WriteString(name)
				res.WriteString(types.SegDelimiter)
				res.WriteString(strconv.Itoa(j))
				seg := stripe[j][d.prefixSize : d.prefixSize+int(d.SegSize)]
				err := d.dv.Input([]byte(res.String()), seg, tag)
				if err != nil {
					// tag is wrong
					stripe[j] = nil
					good--
				}
			} else {
				err := pdpv2.CheckTag(tag)
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
		return directRead, good, ErrRecoverData
	}

	if name != "" {
		ok := d.dv.Result()
		// verify each chunk
		if !ok {
			good = 0
			for j := 0; j < len(stripe); j++ {
				//if good >= int(d.DataCount) {
				//	stripe[j] = nil
				//	continue
				//}
				if len(stripe[j]) == d.fragmentSize {
					res.Reset()
					res.WriteString(name)
					res.WriteString(types.SegDelimiter)
					res.WriteString(strconv.Itoa(j))
					ok, err := d.VerifyChunk(res.String(), stripe[j])
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

func (d *DataCoder) VerifyChunk(name string, data []byte) (bool, error) {
	if len(data) < d.fragmentSize {
		return false, ErrDataLength
	}

	d.dv.Reset()
	if name != "" {
		seg := data[d.prefixSize : d.prefixSize+int(d.SegSize)]
		tag := data[d.prefixSize+int(d.SegSize) : d.prefixSize+int(d.SegSize)+d.tagSize]
		err := d.dv.Input([]byte(name), seg, tag)
		if err != nil {
			return false, err
		}
	}

	return d.dv.Result(), nil
}

func (d *DataCoder) Recover(name string, stripe [][]byte) error {
	_, scount, err := d.VerifyStripe(name, stripe)
	if err != nil {
		return err
	}

	if scount < int(d.DataCount) {
		return ErrRecoverData
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

func (d *DataCoder) recoverField(name string, stripe [][]byte) error {
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

	if name != "" {
		var res strings.Builder
		d.dv.Reset()
		for i := range fault {
			stripe[i] = make([]byte, 0, d.fragmentSize-d.prefixSize)
			stripe[i] = append(stripe[i], dataGroup[i]...)

			res.Reset()
			res.WriteString(name)
			res.WriteString(types.SegDelimiter)
			res.WriteString(strconv.Itoa(i))

			err := d.dv.Input([]byte(res.String()), dataGroup[i], tagGroup[i])
			if err != nil {
				return err
			}
		}

		ok := d.dv.Result()
		if !ok {
			return ErrRecoverData
		}
	} else {
		for i := range fault {
			err := pdpv2.CheckTag(tagGroup[i])
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
				return ErrDataBroken
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
			return ErrRecoverData
		}

		for j := 0; j < d.chunkCount; j++ {
			if data[j] == nil {
				data[j] = make([]byte, 0)
				data[j] = append(data[j], data[i]...)
			}
		}
		return nil
	default:
		return ErrWrongPolicy
	}
}
