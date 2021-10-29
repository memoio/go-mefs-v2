package types

func NodeTopic(netName string) string { return "/memo/node/" + string(netName) }
func MsgTopic(netName string) string  { return "/memo/msg/" + string(netName) }
