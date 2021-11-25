package aes

import (
	"bytes"
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
	ErrKeySize   = errors.New("keysize must be 32")
	ErrBlockSize = errors.New("blocksize must be an integer which can be divisible by 128")
)

// ContructAesEnc contructs a new aes encrypt
func ContructAesEnc(basekey []byte, objectID, stripeID uint64) (cipher.BlockMode, error) {
	tmpkey := make([]byte, len(basekey)+16)
	copy(tmpkey, basekey[:])
	binary.BigEndian.PutUint64(tmpkey[len(basekey):len(basekey)+8], objectID)
	binary.BigEndian.PutUint64(tmpkey[len(basekey)+8:], stripeID)
	key := blake3.Sum256(tmpkey)

	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	iv := blake3.Sum256(key[:])
	return cipher.NewCBCEncrypter(block, iv[:blockSize]), nil
}

func ContructAesDec(basekey []byte, objectID, stripeID uint64) (cipher.BlockMode, error) {
	tmpkey := make([]byte, len(basekey)+16)
	copy(tmpkey, basekey[:])
	binary.BigEndian.PutUint64(tmpkey[len(basekey):len(basekey)+8], objectID)
	binary.BigEndian.PutUint64(tmpkey[len(basekey)+8:], stripeID)
	key := blake3.Sum256(tmpkey)

	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	iv := blake3.Sum256(key[:])
	return cipher.NewCBCDecrypter(block, iv[:blockSize]), nil
}

// padding data to divide BlockSize
func PKCS5Padding(ciphertext []byte) []byte {
	padding := BlockSize - len(ciphertext)%BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func AesEncrypt(origData, key []byte) ([]byte, error) {
	if len(origData)%BlockSize != 0 {
		return nil, ErrBlockSize
	}
	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	// 目前初始向量vi为key的前blocksize个字节
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

func AesDecrypt(crypted, key []byte) ([]byte, error) {
	if len(crypted)%BlockSize != 0 {
		return nil, ErrBlockSize
	}
	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	// 目前初始向量vi为key的前blocksize个字节
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	return origData, nil
}
