package configuration

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

func PrepareConfiguration(confName string) (*Configuration, error) {
	var err error
	if confName == "" {
		errReason := "Empty file name"
		return nil, errors.Wrap(err, errReason)
	}

	fileNames := strings.Split(confName, ".")
	if len(fileNames) != 2 {
		// @bug: fix cases when we have multiple dots in file name
		// e.g. "conf.json.backup"
		// or ""./cmd/conf.toml"
		errReason := fmt.Sprintf("Bad file name '%s'", confName)
		err := fmt.Errorf(errReason)
		return nil, errors.Wrap(err, errReason)
	}
	fileFormat := fileNames[1]

	switch fileFormat {
	case "json":
		mainCfg, err := PrepareConfigurationJSON(confName)
		if err != nil {
			return nil, err
		}
		return mainCfg, nil
	case "toml":
		mainCfg, err := PrepareConfigurationTOML(confName)
		if err != nil {
			return nil, err
		}
		return mainCfg, nil
	default:
		errReason := fmt.Sprintf("Not supported file format '%s'", fileFormat)
		return nil, errors.Wrap(err, errReason)
	}
}
