package committee

type Committee struct {
	members   []uint64
	threshold int
	view      uint64
}

func CalcThreshold(size int) int {
	return (size + 1) * 2 / 3
}

// fair leader rotation
func (c *Committee) GetLeader(view uint64) uint64 {
	return c.members[view%uint64(len(c.members))]
}

func (c *Committee) GetQuorumSize() int {
	return c.threshold
}

func (c *Committee) Size() int {
	return len(c.members)
}

func (c *Committee) UpdateCommittee(uint64) {
	// todo: update members
}

func (c *Committee) UpdateViewNumber(viewN uint64) {
	c.view = viewN
}
