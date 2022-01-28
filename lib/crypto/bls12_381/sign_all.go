package bls

import (
	"encoding/hex"

	"golang.org/x/xerrors"
)

const (
	SignatureSize = 96
	PublicKeySize = 48
	SecretKeySize = 32
)

var (
	errZeroSecretKey     = xerrors.New("zero secret key")
	errZeroPublicKey     = xerrors.New("zero public key")
	errInfinitePublicKey = xerrors.New("infinite public key")
	errInvalidPublicKey  = xerrors.New("invalid public key")
	errZeroSignature     = xerrors.New("zero bls signature")
	errInfiniteSignature = xerrors.New("infinite bls signature")
	errInvalidSignature  = xerrors.New("invalid bls signature")
	errSecretKeySize     = xerrors.New("invalid bls secret key size")
	errInvalidSecretKey  = xerrors.New("invalid bls secret key")
	errPublicKeySize     = xerrors.New("invalid bls public key size")
	errSignatureSize     = xerrors.New("invalid bls signature size")
)

var zeroSecretKey = make([]byte, SecretKeySize)
var zeroPublicKey = make([]byte, PublicKeySize)
var infinitePublicKey []byte
var zeroSignature = make([]byte, SignatureSize)
var infiniteSignature []byte

var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

func initSign() {
	var err error
	infinitePublicKeyStr := "c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	infinitePublicKey, err = hex.DecodeString(infinitePublicKeyStr)
	if err != nil {
		panic("cannot set infinite public key")
	}
	infiniteSignatureStr := "c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	infiniteSignature, err = hex.DecodeString(infiniteSignatureStr)
	if err != nil {
		panic("cannot set infinite signature")
	}
}
