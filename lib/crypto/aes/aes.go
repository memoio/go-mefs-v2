package aes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"

	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

const (
	KeySize   = 32 // 256bit，32B
	BlockSize = 16 // 128bit，16B
)

func ContructAesKey(basekey []byte, bucketID, objectID, stripeID uint64) []byte {
	tmpkey := make([]byte, len(basekey)+8)
	copy(tmpkey, basekey)
	binary.BigEndian.PutUint64(tmpkey[len(basekey):], bucketID)
	hres := blake3.Sum256(tmpkey)

	tmpkey = make([]byte, 48)
	copy(tmpkey, hres[:])
	binary.BigEndian.PutUint64(tmpkey[len(hres):len(hres)+8], objectID)
	binary.BigEndian.PutUint64(tmpkey[len(hres)+8:], stripeID)
	key := blake3.Sum256(tmpkey)
	return key[:]
}

// ContructAesEnc contructs a new aes encrypt
func ContructAesEnc(key []byte) (cipher.BlockMode, error) {
	if len(key) != KeySize {
		return nil, xerrors.New("keysize must be 32")
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	iv := blake3.Sum256(key[:])
	return cipher.NewCBCEncrypter(block, iv[:blockSize]), nil
}

func ContructAesDec(key []byte) (cipher.BlockMode, error) {
	if len(key) != KeySize {
		return nil, xerrors.New("keysize must be 32")
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
		return nil, xerrors.New("blocksize must be an integer which can be divisible by 128")
	}
	if len(key) != KeySize {
		return nil, xerrors.New("keysize must be 32")
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
		return nil, xerrors.New("blocksize must be an integer which can be divisible by 128")
	}
	if len(key) != KeySize {
		return nil, xerrors.New("keysize must be 32")
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
