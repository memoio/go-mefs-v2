package config

import (
	"testing"

	"github.com/mitchellh/go-homedir"
)

func TestConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	cfgName, _ := homedir.Expand("~/test/config1")

	err := cfg.WriteFile(cfgName)
	if err != nil {
		t.Fatal(err)
	}

	cfg.Set("swarm.EnableRelay", "true")

	cfgName2, _ := homedir.Expand("~/test/config2")

	err = cfg.WriteFile(cfgName2)
	if err != nil {
		t.Fatal(err)
	}
}
