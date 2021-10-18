package aes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
)

// key: hash(privatekey+bucketid)
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

// padding data to divide BlockSize
func PKCS5Padding(ciphertext []byte) []byte {
	padding := BlockSize - len(ciphertext)%BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}
