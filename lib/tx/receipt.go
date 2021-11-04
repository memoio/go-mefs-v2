package tx

type ErrCode int64

// need?
type Receipt struct {
	Err   ErrCode
	Extra []byte
}
