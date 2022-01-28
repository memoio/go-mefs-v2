package tx

import "fmt"

type ErrCode uint16

// need?
type Receipt struct {
	Err   ErrCode
	Extra string
}

func (r Receipt) String() string {
	return fmt.Sprintf("receipt result %d: %s", r.Err, r.Extra)
}
