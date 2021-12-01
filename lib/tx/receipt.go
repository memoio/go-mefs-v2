package tx

type ErrCode uint16

// need?
type Receipt struct {
	Err   ErrCode
	Extra []byte
}
