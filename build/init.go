package build

var UpdateMap map[uint32]uint64
var ChalDurMap map[uint32]uint64

func init() {
	UpdateMap = make(map[uint32]uint64)
	UpdateMap[1] = UpdateHeight1
	UpdateMap[2] = UpdateHeight2
	UpdateMap[3] = UpdateHeight3

	ChalDurMap = make(map[uint32]uint64)
	ChalDurMap[0] = ChalDuration0
	ChalDurMap[1] = ChalDuration1
	ChalDurMap[2] = ChalDuration2
	ChalDurMap[3] = ChalDuration3
}
