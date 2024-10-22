package configuration

import (
	"encoding/json"
	"os"
)

func PrepareConfigurationJSON(fname string) (*Configuration, error) {
	configFile, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	cfg := &Configuration{}
	err = json.Unmarshal(configFile, &cfg)
	if err != nil {
		return nil, err
	}
	postProcessDefaults(cfg)
	return cfg, nil
}
