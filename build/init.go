package build

var UpdateMap map[uint32]uint64
var ChalDurMap map[uint32]uint64

func init() {
	UpdateMap = make(map[uint32]uint64)
	UpdateMap[1] = UpdateEpoch1

	ChalDurMap = make(map[uint32]uint64)
	ChalDurMap[0] = ChalDuration0
	ChalDurMap[1] = ChalDuration1
}
