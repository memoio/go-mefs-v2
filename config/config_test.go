package config

import "testing"

func TestConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	err := cfg.WriteFile("/home/fjt/test/config1")
	if err != nil {
		t.Fatal(err)
	}

	cfg.Set("swarm.EnableRelay", "true")

	err = cfg.WriteFile("/home/fjt/test/config2")
	if err != nil {
		t.Fatal(err)
	}
}
