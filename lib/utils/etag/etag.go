package etag

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

func ToString(etag []byte) (string, error) {
	if len(etag) == md5.Size {
		return hex.EncodeToString(etag), nil
	}

	_, ecid, err := cid.CidFromBytes(etag)
	if err != nil {
		return "", err
	}

	return ecid.String(), nil
}

func ToCidV0String(etag []byte) (string, error) {
	if len(etag) == md5.Size {
		return "", xerrors.Errorf("invalid cid format")
	}

	_, ecid, err := cid.CidFromBytes(etag)
	if err != nil {
		return "", err
	}
	if len(etag) > 2 && etag[0] == mh.SHA2_256 && etag[1] == 32 {
		return ecid.String(), nil
	}

	// change it to v0 string
	return cid.NewCidV0(ecid.Hash()).String(), nil
}
