package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"

	"github.com/zeebo/blake3"
)

const (
	KeySize   = 32 // 256bit，32B
	BlockSize = 16 // 128bit，16B
)

var (
	ErrKeySize   = errors.New("Keysize must be 32")
	ErrBlockSize = errors.New("Blocksize must be an integer which can be divisible by 128")
)

// CreateAesKey creates
func CreateAesKey(privateKey, queryID []byte, bucketID, objectStart int64) [32]byte {
	tmpkey := make([]byte, len(privateKey)+len(queryID)+16)
	copy(tmpkey, privateKey)
	copy(tmpkey[len(privateKey):], queryID)
	binary.LittleEndian.PutUint64(tmpkey[len(privateKey)+len(queryID):], uint64(bucketID))
	binary.LittleEndian.PutUint64(tmpkey[len(privateKey)+len(queryID)+8:], uint64(objectStart))
	return blake3.Sum256(tmpkey)
}

// ContructAesEnc contructs a new aes encrypt
func ContructAesEnc(key []byte) (cipher.BlockMode, error) {
	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	iv := blake3.Sum256(key)
	return cipher.NewCBCEncrypter(block, iv[:blockSize]), nil
}

func ContructAesDec(key []byte) (cipher.BlockMode, error) {
	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	iv := blake3.Sum256(key)
	return cipher.NewCBCDecrypter(block, iv[:blockSize]), nil
}
