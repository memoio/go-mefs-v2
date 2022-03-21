package build

var UpdateMap map[uint32]uint64
var ChalDurMap map[uint32]uint64
var OrderDurMap map[uint32]int64

func init() {
	UpdateMap = make(map[uint32]uint64)
	UpdateMap[1] = UpdateHeight1
	UpdateMap[2] = UpdateHeight2

	ChalDurMap = make(map[uint32]uint64)
	ChalDurMap[0] = ChalDuration0
	ChalDurMap[1] = ChalDuration1
	ChalDurMap[2] = ChalDuration2

	OrderDurMap = make(map[uint32]int64)
	OrderDurMap[0] = OrderMin0
	OrderDurMap[1] = OrderMin1
	OrderDurMap[2] = OrderMin2
}
