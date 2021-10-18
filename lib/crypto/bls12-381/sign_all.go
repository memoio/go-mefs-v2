package bls

import (
	"encoding/hex"
	"errors"
)

const (
	SignatureSize = 96
	PublicKeySize = 48
	SecretKeySize = 32
)

var (
	errZeroSecretKey     = errors.New("zero secret key")
	errZeroPublicKey     = errors.New("zero public key")
	errInfinitePublicKey = errors.New("infinite public key")
	errInvalidPublicKey  = errors.New("invalid public key")
	errZeroSignature     = errors.New("zero signature")
	errInfiniteSignature = errors.New("infinite signature")
	errInvalidSignature  = errors.New("invalid signature")
	errSecretKeySize     = errors.New("invalid secret key size")
	errInvalidSecretKey  = errors.New("invalid secret key")
	errPublicKeySize     = errors.New("invalid public key size")
	errSignatureSize     = errors.New("invalid signature size")
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
