package aes

import (
	"crypto/aes"
	"crypto/cipher"
)

// key: hash(privatekey+bucketid)
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

func PKCS5UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}
