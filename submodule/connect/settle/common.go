package settle

import (
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/lib/address"
)

// for test
func RegisterRole(addr address.Address) (uint64, error) {
	return binary.BigEndian.Uint64(addr.Bytes()), nil
}

func GetRoleID(addr address.Address) (uint64, error) {
	return binary.BigEndian.Uint64(addr.Bytes()), nil
}

func GetGroupID(roleID uint64) uint64 {
	return 1
}

func GetThreshold(groupID uint64) int {
	return 1
}
