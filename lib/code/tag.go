package code

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
)

//GenTagForSegment 根据指定段大小生成标签，index是生成BLS-tag的需要
func (d *DataCoder) GenTag(index, data []byte) ([]byte, error) {
	switch d.Prefix.TagFlag {
	case pdp.CRC32:
		return uint32ToBytes(crc32.ChecksumIEEE(data)), nil
	case pdp.BLS:
		return nil, ErrWrongTagFlag
	case pdp.PDPV0:
		return nil, ErrWrongTagFlag
	case pdp.PDPV2:
		res, err := d.blsKey.GenTag(index, data, 0, true)
		if err != nil {
			return nil, err
		}
		return res, nil
	default:
		return nil, ErrWrongTagFlag
	}
}

//将uint32切片转成[]byte
func uint32ToBytes(vs uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, vs)
	return buf
}
