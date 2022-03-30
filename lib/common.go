package lib

import (
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/types"

	"golang.org/x/xerrors"
)

func CheckOrder(or types.OrderBase) error {
	if or.SegPrice == nil {
		return xerrors.Errorf("empty seg price")
	}

	if or.PiecePrice == nil {
		return xerrors.Errorf("empty piece price")
	}

	/*
		if or.End-or.Start < build.OrderMin {
			return xerrors.Errorf("order duration %d is short than %d", or.End-or.Start, build.OrderMin)
		}
	*/

	if or.Start <= build.BaseTime || or.End <= build.BaseTime {
		return xerrors.Errorf("order start/end should greater than %d", build.BaseTime)
	}

	if or.End-or.Start > build.OrderMax {
		return xerrors.Errorf("order duration %d is greater than %d", or.End-or.Start, build.OrderMax)
	}

	if or.End%types.Day != 0 {
		return xerrors.Errorf("order end %d is not aligned", or.End)
	}

	return nil
}
