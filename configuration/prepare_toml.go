package configuration

import (
	"github.com/BurntSushi/toml"
)

func PrepareConfigurationTOML(fname string) (*Configuration, error) {
	cfg := &Configuration{}
	_, err := toml.DecodeFile(fname, cfg)
	if err != nil {
		return nil, err
	}
	postProcessDefaults(cfg)
	return cfg, nil
}
