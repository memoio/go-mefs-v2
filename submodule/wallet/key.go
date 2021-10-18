package wallet

import (
	"bytes"
	"crypto/aes"
	cr "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"golang.org/x/crypto/scrypt"
)

//Key the struct privatekey transform to
type Key struct {
	Id uuid.UUID

	Type byte
	// to simplify lookups we also store the address
	Address []byte
	// we only store privkey as pubkey/address can be derived from it
	// privkey in this struct is always in plaintext
	// Eth format: 66bytes
	PrivateKey []byte
}

type keyStore interface {
	// Loads and decrypts the key from disk.
	GetKey(addr []byte, filename, auth string) (*Key, error)
	// Writes and encrypts the key.
	StoreKey(filename string, k *Key, auth string) error
	// Joins filename with the key directory unless it is already absolute.
	JoinPath(filename string) string
}

type cipherparamsJSON struct {
	IV string `json:"iv"`
}

type CryptoJSON struct {
	Cipher       string                 `json:"cipher"`
	CipherText   string                 `json:"ciphertext"`
	CipherParams cipherparamsJSON       `json:"cipherparams"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

type encryptedKeyJSONV3 struct {
	Address string     `json:"address"`
	Crypto  CryptoJSON `json:"crypto"`
	Type    byte       `json:"type"`
	Id      string     `json:"id"`
	Version int        `json:"version"`
}

var (
	//ErrDecrypt before decrypt privatekey, we compare mac, if not equal, use ErrDecrypt
	ErrDecrypt = errors.New("could not decrypt key with given passphrase")
)

func newKey(privatekey, address []byte) (*Key, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	key := &Key{
		Id:         id,
		Address:    address,
		PrivateKey: privatekey,
	}
	return key, nil
}

// StorePrivateKey encrypt the privatekey by password and then store it in keystore
func StorePrivateKey(dir string, privatekey sig_common.PrivKey, password string) error {
	if privatekey == nil {
		return ErrDecrypt
	}
	pubkey := privatekey.GetPublic()

	addr, err := signature.GetAdressFromPubkey(pubkey)
	if err != nil {
		return err
	}

	path := joinPath(dir, hex.EncodeToString(addr))
	_, err = os.Stat(path)
	if os.IsExist(err) {
		return nil
	}

	keyBytes, err := privatekey.Raw()
	if err != nil {
		return err
	}
	key, err := newKey(keyBytes, addr)
	if err != nil {
		return err
	}
	key.Type = privatekey.Type()

	keyjson, err := encryptKey(key, password, StandardScryptN, StandardScryptP)
	if err != nil {
		return err
	}

	return writeKeyFile(path, keyjson) //写入文件
}

func LoadPrivateKey(address string, password, p string) (privatekey sig_common.PrivKey, err error) {
	if address[:2] == "0x" {
		address = address[2:]
	}

	addr, err := hex.DecodeString(address)
	if err != nil {
		return nil, err
	}

	// Load the key from the keystore and decrypt its contents
	keyjson, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	key, err := decryptKey(keyjson, password)
	if err != nil {
		return nil, err
	}
	// Make sure we're really operating on the requested key (no swap attacks)
	if !bytes.Equal(key.Address, addr) {
		return nil, fmt.Errorf("key content mismatch: have peer %x, want %x", key.Address, address)
	}

	return signature.ParsePrivateKey(key.PrivateKey, key.Type)
}

func joinPath(dir string, filename string) (path string) {
	if filepath.IsAbs(filename) {
		return filename
	}
	return filepath.Join(dir, filename)
}

// EncryptKey encrypts a key using the specified scrypt parameters into a json
// blob that can be decrypted later on.
func encryptKey(key *Key, password string, scryptN, scryptP int) ([]byte, error) {
	passwordArray := []byte(password)
	salt := getEntropyCSPRNG(32)                                                               //生成一个随即的32B的salt
	derivedKey, err := scrypt.Key(passwordArray, salt, scryptN, scryptR, scryptP, scryptDKLen) //使用scrypt算法对输入的password加密，生成一个32位的derivedKey
	if err != nil {
		return nil, err
	}
	encryptKey := derivedKey[:16] //why

	iv := getEntropyCSPRNG(aes.BlockSize)                        // 16,aes-128-ctr加密算法需要的初始化向量
	cipherText, err := aesCTRXOR(encryptKey, key.PrivateKey, iv) //对privatekey进行aes加密，生成一个32byte的cipherText
	if err != nil {
		return nil, err
	}
	mac := crypto.Keccak256(derivedKey[16:32], cipherText) //将derivedKey的后16byte与cipherText进行Keccak256哈希，生成32byte的mac，mac用于验证解密时password的正确性

	scryptParamsJSON := make(map[string]interface{}, 5)
	scryptParamsJSON["n"] = scryptN
	scryptParamsJSON["r"] = scryptR
	scryptParamsJSON["p"] = scryptP
	scryptParamsJSON["dklen"] = scryptDKLen
	scryptParamsJSON["salt"] = hex.EncodeToString(salt)

	cipherParamsJSON := cipherparamsJSON{
		IV: hex.EncodeToString(iv),
	}

	cryptoStruct := CryptoJSON{
		Cipher:       "aes-128-ctr",
		CipherText:   hex.EncodeToString(cipherText),
		CipherParams: cipherParamsJSON,
		KDF:          keyHeaderKDF,
		KDFParams:    scryptParamsJSON,
		MAC:          hex.EncodeToString(mac),
	}
	encryptedKeyJSONV3 := encryptedKeyJSONV3{
		hex.EncodeToString(key.Address),
		cryptoStruct,
		key.Type,
		key.Id.String(),
		latestVersion,
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

// decryptKey decrypts a key from a json blob, returning the private key itself.
func decryptKey(keyjson []byte, password string) (*Key, error) {
	// Depending on the version try to parse one way or another
	var (
		keyBytes []byte
		keyID    []byte
		err      error
	)
	k := new(encryptedKeyJSONV3)
	if err := json.Unmarshal(keyjson, k); err != nil {
		return nil, err
	}
	keyBytes, keyID, err = decryptKeyV3(k, password)
	// Handle any decryption errors and return the key
	if err != nil {
		return nil, err
	}
	var key sig_common.PrivKey
	var pubkey sig_common.PubKey
	switch k.Type {
	case sig_common.BLS:
		key = &bls.PrivateKey{}
		key.Deserialize(keyBytes)
		pubkey = key.GetPublic()
	case sig_common.Secp256k1:
		key = &secp256k1.PrivateKey{}
		key.Deserialize(keyBytes)
		pubkey = key.GetPublic()
	}

	if key == nil {
		return nil, ErrDecrypt
	}

	id, err := uuid.FromBytes(keyID)
	if err != nil {
		return nil, err
	}

	addr, err := signature.GetAdressFromPubkey(pubkey)
	if err != nil {
		return nil, err
	}

	return &Key{
		Id:         id,
		Type:       k.Type,
		Address:    addr,
		PrivateKey: keyBytes,
	}, nil
}

func writeTemporaryKeyFile(file string, content []byte) (string, error) {
	// Create the keystore directory with appropriate permissions
	// in case it is not present yet.
	const dirPerm = 0700
	if err := os.MkdirAll(filepath.Dir(file), dirPerm); err != nil {
		return "", err
	}
	// Atomic write: create a temporary hidden file first
	// then move it into place. TempFile assigns mode 0600.
	f, err := ioutil.TempFile(filepath.Dir(file), "."+filepath.Base(file)+".tmp")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

func writeKeyFile(file string, content []byte) error {
	name, err := writeTemporaryKeyFile(file, content)
	if err != nil {
		return err
	}
	return os.Rename(name, file)
}

// keyFileName implements the naming convention for keyfiles:
// UTC--<created_at UTC ISO8601>-<address hex>
func keyFileName(keyAddr []byte) string {
	ts := time.Now().UTC()
	return fmt.Sprintf("UTC--%s--%s", toISO8601(ts), hex.EncodeToString(keyAddr[:]))
}

func toISO8601(t time.Time) string {
	var tz string
	name, offset := t.Zone()
	if name == "UTC" {
		tz = "Z"
	} else {
		tz = fmt.Sprintf("%03d00", offset/3600)
	}
	return fmt.Sprintf("%04d-%02d-%02dT%02d-%02d-%02d.%09d%s",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), tz)
}
