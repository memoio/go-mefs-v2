package settle

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"

	"github.com/memoio/go-mefs-v2/lib/address"
)

var (
	roleAddr = common.HexToAddress("0x7a424f9aF3A69e19fe2A839Cf564d620B6C984d7")
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
	return 7
}
