package types

import (
	"errors"
)

var (
	ErrKeyInfoNotFound = errors.New("key info not found")
	ErrKeyExists       = errors.New("key already exists")
	ErrKeyFromat       = errors.New("key format is wrong")
)

type KeyType = byte

const (
	Unknown KeyType = iota
	// network
	P2P
	// data verify key
	BLS12
	// verify key on chain
	SECP256K1
	// user/keeper/provider/fs
	ACTOR
)

// KeyInfo is used for storing keys in KeyStore
type KeyInfo struct {
	Type      KeyType
	SecretKey []byte
}

// KeyStore is used for storing secret keys
type KeyStore interface {
	// List lists all the keys stored in the KeyStore
	List() ([]string, error)
	// Get gets a key out of keystore use its name and password
	Get(string, string) (KeyInfo, error)
	// Put saves a key info under given name and password
	Put(string, string, KeyInfo) error
	// Delete removes a key from keystores
	Delete(string, string) error
}
