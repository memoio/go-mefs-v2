package types

import (
	"errors"
)

var (
	ErrKeyInfoNotFound = errors.New("key info not found")
	ErrKeyExists       = errors.New("key already exists")
	ErrKeyFromat       = errors.New("key format is wrong")
)

// KeyInfo is used for storing keys in KeyStore
type KeyInfo struct {
	Type      byte
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
