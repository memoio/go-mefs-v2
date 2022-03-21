package keystore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	cr "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/xerrors"
)

const (
	keyHeaderKDF = "scrypt"
	// StandardScryptN is the N parameter of Scrypt encryption algorithm, using 256MB
	// memory and taking approximately 1s CPU time on a modern processor.
	StandardScryptN = 1 << 18
	// StandardScryptP is the P parameter of Scrypt encryption algorithm, using 256MB
	// memory and taking approximately 1s CPU time on a modern processor.
	StandardScryptP = 1
	scryptR         = 8
	scryptDKLen     = 32
	version         = 3
)

type Key struct {
	Address     string
	SecretValue []byte
}

type cipherparamsJSON struct {
	IV string `json:"iv"`
}

type cryptoJSON struct {
	Cipher       string                 `json:"cipher"`
	CipherText   string                 `json:"ciphertext"`
	CipherParams cipherparamsJSON       `json:"cipherparams"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

type encryptedKeyJSONV3 struct {
	Address string     `json:"address"`
	Crypto  cryptoJSON `json:"crypto"`
	Version int        `json:"version"`
}

// encryptKey encrypts a key using the specified scrypt parameters into a json
// blob that can be decrypted later on.
func encryptKey(key *Key, password string, scryptN, scryptP int) ([]byte, error) {
	passwordArray := []byte(password)
	salt := getEntropyCSPRNG(32)                                                               //生成一个随即的32B的salt
	derivedKey, err := scrypt.Key(passwordArray, salt, scryptN, scryptR, scryptP, scryptDKLen) //使用scrypt算法对输入的password加密，生成一个32位的derivedKey
	if err != nil {
		return nil, err
	}
	encryptKey := derivedKey[:16]

	iv := getEntropyCSPRNG(aes.BlockSize)                         // 16,aes-128-ctr加密算法需要的初始化向量
	cipherText, err := aesCTRXOR(encryptKey, key.SecretValue, iv) //对privatekey进行aes加密，生成一个32byte的cipherText
	if err != nil {
		return nil, err
	}

	//将derivedKey的后16byte与cipherText进行Keccak256哈希，生成32byte的mac，mac用于验证解密时password的正确性
	d := blake3.New()
	d.Write(derivedKey[16:32])
	d.Write(cipherText)
	mac := d.Sum(nil)

	scryptParamsJSON := make(map[string]interface{}, 5)
	scryptParamsJSON["n"] = scryptN
	scryptParamsJSON["r"] = scryptR
	scryptParamsJSON["p"] = scryptP
	scryptParamsJSON["dklen"] = scryptDKLen
	scryptParamsJSON["salt"] = hex.EncodeToString(salt)

	cipherParamsJSON := cipherparamsJSON{
		IV: hex.EncodeToString(iv),
	}

	cryptoStruct := cryptoJSON{
		Cipher:       "aes-128-ctr",
		CipherText:   hex.EncodeToString(cipherText),
		CipherParams: cipherParamsJSON,
		KDF:          keyHeaderKDF,
		KDFParams:    scryptParamsJSON,
		MAC:          hex.EncodeToString(mac),
	}
	encryptedKeyJSONV3 := encryptedKeyJSONV3{
		key.Address,
		cryptoStruct,
		version,
	}
	return json.Marshal(encryptedKeyJSONV3)
}

func getEntropyCSPRNG(n int) []byte {
	mainBuff := make([]byte, n)
	_, err := io.ReadFull(cr.Reader, mainBuff)
	if err != nil {
		panic("reading from crypto/rand failed: " + err.Error())
	}
	return mainBuff
}

func aesCTRXOR(key, inText, iv []byte) ([]byte, error) {
	// AES-128 is selected due to size of encryptKey.
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(aesBlock, iv)
	outText := make([]byte, len(inText))
	stream.XORKeyStream(outText, inText)
	return outText, err
}

func decryptKey(keyjson []byte, auth string) (*Key, error) {
	k := new(encryptedKeyJSONV3)
	err := json.Unmarshal(keyjson, k)
	if err != nil {
		return nil, err
	}

	// Handle any decryption errors and return the key
	keyBytes, err := decryptKeyV3(k, auth)
	if err != nil {
		return nil, err
	}

	return &Key{
		Address:     k.Address,
		SecretValue: keyBytes,
	}, nil
}

func decryptKeyV3(keyProtected *encryptedKeyJSONV3, password string) (keyBytes []byte, err error) {
	if keyProtected.Version != version {
		return nil, xerrors.Errorf("version not supported: %v", keyProtected.Version)
	}

	if keyProtected.Crypto.Cipher != "aes-128-ctr" {
		return nil, xerrors.Errorf("cipher not supported: %v", keyProtected.Crypto.Cipher)
	}

	mac, err := hex.DecodeString(keyProtected.Crypto.MAC)
	if err != nil {
		return nil, err
	}

	iv, err := hex.DecodeString(keyProtected.Crypto.CipherParams.IV)
	if err != nil {
		return nil, err
	}

	cipherText, err := hex.DecodeString(keyProtected.Crypto.CipherText)
	if err != nil {
		return nil, err
	}

	derivedKey, err := getKDFKey(keyProtected.Crypto, password)
	if err != nil {
		return nil, err
	}

	d := blake3.New()
	d.Write(derivedKey[16:32])
	d.Write(cipherText)
	calculatedMAC := d.Sum(nil)
	if !bytes.Equal(calculatedMAC, mac) {
		return nil, xerrors.New("could not decrypt key with given passphrase")
	}

	plainText, err := aesCTRXOR(derivedKey[:16], cipherText, iv)
	if err != nil {
		return nil, err
	}

	return plainText, err
}

func getKDFKey(cryptoJSON cryptoJSON, password string) ([]byte, error) {
	passwordArray := []byte(password)
	salt, err := hex.DecodeString(cryptoJSON.KDFParams["salt"].(string))
	if err != nil {
		return nil, err
	}
	dkLen := ensureInt(cryptoJSON.KDFParams["dklen"])

	if cryptoJSON.KDF == keyHeaderKDF {
		n := ensureInt(cryptoJSON.KDFParams["n"])
		r := ensureInt(cryptoJSON.KDFParams["r"])
		p := ensureInt(cryptoJSON.KDFParams["p"])
		return scrypt.Key(passwordArray, salt, n, r, p, dkLen)

	}
	return nil, xerrors.Errorf("unsupported KDF: %s", cryptoJSON.KDF)
}

func ensureInt(x interface{}) int {
	res, ok := x.(int)
	if !ok {
		res = int(x.(float64))
	}
	return res
}
