package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

// Validators hold the list of validation functions for each configuration
// property. Validators must take a key and json string respectively as
// arguments, and must return either an error or nil depending on whether or not
// the given key and value are valid. Validators will only be run if a property
// being set matches the name given in this map.
var Validators = map[string]func(string, string) error{
	"heartbeat.nickname": validateLettersOnly,
}

// Config is an in memory representation of the filecoin configuration file
type Config struct {
	Identity  IdentityConfig  `json:"identity"`
	Wallet    WalletConfig    `json:"wallet"`
	Net       SwarmConfig     `json:"net"`
	API       APIConfig       `json:"api"`
	Bootstrap BootstrapConfig `json:"bootstrap"`
	Data      StorePathConfig `json:"data"`
}

type WalletConfig struct {
	DefaultAddress string `json:"defaultAddress,omitempty"`
}

type IdentityConfig struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func newDefaultIdentityConfig() IdentityConfig {
	return IdentityConfig{}
}

// APIConfig holds all configuration options related to the api.
// nolint
type APIConfig struct {
	APIAddress                    string   `json:"apiAddress"`
	AccessControlAllowOrigin      []string `json:"accessControlAllowOrigin"`
	AccessControlAllowCredentials bool     `json:"accessControlAllowCredentials"`
	AccessControlAllowMethods     []string `json:"accessControlAllowMethods"`
}

func newDefaultAPIConfig() APIConfig {
	return APIConfig{
		APIAddress: "/ip4/127.0.0.1/tcp/0",
		AccessControlAllowOrigin: []string{
			"http://localhost:8080",
			"https://localhost:8080",
			"http://127.0.0.1:8080",
			"https://127.0.0.1:8080",
		},
		AccessControlAllowMethods: []string{"GET", "POST", "PUT"},
	}
}

// BootstrapConfig holds all configuration options related to bootstrap nodes
type BootstrapConfig struct {
	Addresses []string `json:"addresses"`
}

// TODO: provide bootstrap node addresses
func newDefaultBootstrapConfig() BootstrapConfig {
	return BootstrapConfig{
		Addresses: []string{},
	}
}

type NetworkParamsConfig struct {
	NetName string `json:"netName"`
}

func newDefaultNetworkParamsConfig() NetworkParamsConfig {
	return NetworkParamsConfig{
		NetName: "beta",
	}
}

type StorePathConfig struct {
	MetaPath        string   `json:"metaPath"`
	DataPath        string   `json:"dataPath"`
	VolumeIndexPath string   `json:"volumeIndexPath"`
	VolumeDataPath  []string `json:"volumeDataPath"`
}

// TODO: provide bootstrap node addresses
func newDefaultStorePathConfig() StorePathConfig {
	return StorePathConfig{}
}

// NewDefaultConfig returns a config object with all the fields filled out to
// their default values
func NewDefaultConfig() *Config {

	return &Config{
		Identity:  newDefaultIdentityConfig(),
		API:       newDefaultAPIConfig(),
		Bootstrap: newDefaultBootstrapConfig(),
		Data:      newDefaultStorePathConfig(),
		Net:       newDefaultSwarmConfig(),
	}
}

// WriteFile writes the config to the given filepath.
func (cfg *Config) WriteFile(file string) error {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close() // nolint: errcheck

	configString, err := json.MarshalIndent(*cfg, "", "\t")
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(f, string(configString))
	return err
}

// ReadFile reads a config file from disk.
func ReadFile(file string) (*Config, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	cfg := NewDefaultConfig()
	rawConfig, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if len(rawConfig) == 0 {
		return cfg, nil
	}

	err = json.Unmarshal(rawConfig, &cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// Set sets the config sub-struct referenced by `key`, e.g. 'api.address'
// or 'datastore' to the json key value pair encoded in jsonVal.
func (cfg *Config) Set(dottedKey string, jsonString string) error {
	if !json.Valid([]byte(jsonString)) {
		jsonBytes, _ := json.Marshal(jsonString)
		jsonString = string(jsonBytes)
	}

	if err := validate(dottedKey, jsonString); err != nil {
		return err
	}

	keys := strings.Split(dottedKey, ".")
	for i := len(keys) - 1; i >= 0; i-- {
		jsonString = fmt.Sprintf(`{ "%s": %s }`, keys[i], jsonString)
	}

	decoder := json.NewDecoder(strings.NewReader(jsonString))
	decoder.DisallowUnknownFields()

	return decoder.Decode(&cfg)
}

// Get gets the config sub-struct referenced by `key`, e.g. 'api.address'
func (cfg *Config) Get(key string) (interface{}, error) {
	v := reflect.Indirect(reflect.ValueOf(cfg))
	keyTags := strings.Split(key, ".")
OUTER:
	for j, keyTag := range keyTags {
		if v.Type().Kind() == reflect.Struct {
			for i := 0; i < v.NumField(); i++ {
				jsonTag := strings.Split(
					v.Type().Field(i).Tag.Get("json"),
					",")[0]
				if jsonTag == keyTag {
					v = v.Field(i)
					if j == len(keyTags)-1 {
						return v.Interface(), nil
					}
					v = reflect.Indirect(v) // only attempt one dereference
					continue OUTER
				}
			}
		}

		return nil, fmt.Errorf("key: %s invalid for config", key)
	}
	// Cannot get here as len(strings.Split(s, sep)) >= 1 with non-empty sep
	return nil, fmt.Errorf("empty key is invalid")
}

// validate runs validations on a given key and json string. validate uses the
// validators map defined at the top of this file to determine which validations
// to use for each key.
func validate(dottedKey string, jsonString string) error {
	var obj interface{}
	if err := json.Unmarshal([]byte(jsonString), &obj); err != nil {
		return err
	}
	// recursively validate sub-keys by partially unmarshalling
	if reflect.ValueOf(obj).Kind() == reflect.Map {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(jsonString), &obj); err != nil {
			return err
		}
		for key := range obj {
			if err := validate(dottedKey+"."+key, string(obj[key])); err != nil {
				return err
			}
		}
		return nil
	}

	if validationFunc, present := Validators[dottedKey]; present {
		return validationFunc(dottedKey, jsonString)
	}

	return nil
}

// validateLettersOnly validates that a given value contains only letters. If it
// does not, an error is returned using the given key for the message.
func validateLettersOnly(key string, value string) error {
	if match, _ := regexp.MatchString("^\"[a-zA-Z]+\"$", value); !match {
		return errors.Errorf(`"%s" must only contain letters`, key)
	}
	return nil
}
