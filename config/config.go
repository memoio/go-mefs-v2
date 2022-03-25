package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"golang.org/x/xerrors"
)

var Validators = map[string]func(string, string) error{
	"heartbeat.nickname": validateLettersOnly,
}

type Config struct {
	Identity  IdentityConfig  `json:"identity"`
	Wallet    WalletConfig    `json:"wallet"`
	Net       SwarmConfig     `json:"net"`
	API       APIConfig       `json:"api"`
	Bootstrap BootstrapConfig `json:"bootstrap"`
	Data      StorePathConfig `json:"data"`
	Contract  ContractConfig  `json:"contract"`
	Order     OrderConfig     `json:"order"`
}

type ContractConfig struct {
	EndPoint     string `json:"endPoint"`
	RoleContract string `json:"roleContract"`
}

func newDefaultContractConfig() ContractConfig {
	return ContractConfig{
		EndPoint:     settle.EndPoint,
		RoleContract: settle.RoleContract,
	}
}

type WalletConfig struct {
	DefaultAddress string `json:"address,omitempty"`
}

type IdentityConfig struct {
	Role  string `json:"role"`
	Group string `json:"group"`
}

func newDefaultIdentityConfig() IdentityConfig {
	return IdentityConfig{
		Role:  "user",
		Group: "group",
	}
}

type SwarmConfig struct {
	Name string `json:"name"`
	// addresses for the swarm to listen on
	Addresses []string `json:"addresses"`

	EnableRelay bool `json:"enableRelay"`

	PublicRelayAddress string `json:"public_relay_address,omitempty"`
}

func newDefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		Name: "devnet",
		Addresses: []string{
			"/ip4/0.0.0.0/tcp/7000",
			"/ip6/::/tcp/7001",
			"/ip4/0.0.0.0/udp/7000/quic",
			"/ip6/::/udp/7001/quic",
		},
	}
}

type APIConfig struct {
	Address                       string   `json:"address"`
	AccessControlAllowOrigin      []string `json:"accessControlAllowOrigin"`
	AccessControlAllowCredentials bool     `json:"accessControlAllowCredentials"`
	AccessControlAllowMethods     []string `json:"accessControlAllowMethods"`
}

func newDefaultAPIConfig() APIConfig {
	return APIConfig{
		Address: "/ip4/127.0.0.1/tcp/5001",
		AccessControlAllowOrigin: []string{
			"http://localhost:5001",
			"https://localhost:5001",
			"http://127.0.0.1:5001",
			"https://127.0.0.1:5001",
		},
		AccessControlAllowMethods: []string{"GET", "POST", "PUT"},
	}
}

type BootstrapConfig struct {
	Addresses []string `json:"addresses"`
}

// TODO: provide bootstrap node addresses
var DefaultBootstrapConfig = BootstrapConfig{
	Addresses: []string{
		"/ip4/121.37.158.192/tcp/23456/p2p/12D3KooWHXmKSneyGqE8fPrTmNTBs2rR9pWTdNcgVG3Tt5htJef7", // group1
		"/ip4/121.36.243.236/tcp/2222/p2p/12D3KooWKHeDhgLExibUcofNPtwMKEsjxf18KmKoyzyRftZiRWLb",  // group2
		"/ip4/192.168.1.46/tcp/4201/p2p/12D3KooWB5yMrUL6NG6wHrdR9V114mUDkpJ5Mp3c1sLPHwiFi6DN",
	},
}

type StorePathConfig struct {
	MetaPath string `json:"metaPath"`
	DataPath string `json:"dataPath"`
}

func newDefaultStorePathConfig() StorePathConfig {
	return StorePathConfig{}
}

type OrderConfig struct {
	Price    uint64 `json:"price"`
	Duration uint64 `json:"duration"` // day
	Wait     uint64 `json:"wait"`     // seconds
}

func newDefaultOrderConfig() OrderConfig {
	return OrderConfig{
		Price:    build.DefaultSegPrice.Uint64(),
		Duration: 100,       // 100 day
		Wait:     3600 * 12, // 12 hour
	}
}

func NewDefaultConfig() *Config {
	return &Config{
		Identity:  newDefaultIdentityConfig(),
		API:       newDefaultAPIConfig(),
		Bootstrap: DefaultBootstrapConfig,
		Data:      newDefaultStorePathConfig(),
		Net:       newDefaultSwarmConfig(),
		Order:     newDefaultOrderConfig(),
	}
}

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

func (cfg *Config) Set(dottedKey string, jsonString string) error {
	if !json.Valid([]byte(jsonString)) {
		jsonBytes, _ := json.Marshal(jsonString)
		jsonString = string(jsonBytes)
	}

	err := validate(dottedKey, jsonString)
	if err != nil {
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

		return nil, xerrors.Errorf("key: %s invalid for config", key)
	}
	// Cannot get here as len(strings.Split(s, sep)) >= 1 with non-empty sep
	return nil, xerrors.Errorf("empty key is invalid")
}

// validate runs validations on a given key and json string. validate uses the
// validators map defined at the top of this file to determine which validations
// to use for each key.
func validate(dottedKey string, jsonString string) error {
	var obj interface{}
	err := json.Unmarshal([]byte(jsonString), &obj)
	if err != nil {
		return err
	}
	// recursively validate sub-keys by partially unmarshalling
	if reflect.ValueOf(obj).Kind() == reflect.Map {
		var obj map[string]json.RawMessage
		err := json.Unmarshal([]byte(jsonString), &obj)
		if err != nil {
			return err
		}
		for key := range obj {
			err := validate(dottedKey+"."+key, string(obj[key]))
			if err != nil {
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
		return xerrors.Errorf(`"%s" must only contain letters`, key)
	}
	return nil
}
