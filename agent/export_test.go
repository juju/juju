// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"

	"github.com/juju/juju/state/multiwatcher"
)

func Password(config Config) string {
	c := config.(*configInternal)
	if c.stateDetails == nil {
		return c.apiDetails.password
	}
	return c.stateDetails.password
}

func PatchConfig(config Config, fieldName string, value interface{}) error {
	conf := config.(*configInternal)
	switch fieldName {
	case "Paths":
		paths := value.(Paths)
		if paths.DataDir != "" {
			conf.paths.DataDir = paths.DataDir
		}
		if paths.LogDir != "" {
			conf.paths.LogDir = paths.LogDir
		}
	case "Jobs":
		conf.jobs = value.([]multiwatcher.MachineJob)[:]
	case "DeleteValues":
		for _, key := range value.([]string) {
			delete(conf.values, key)
		}
	case "Values":
		for key, val := range value.(map[string]string) {
			if conf.values == nil {
				conf.values = make(map[string]string)
			}
			conf.values[key] = val
		}
	default:
		return fmt.Errorf("unknown field %q", fieldName)
	}
	conf.configFilePath = ConfigPath(conf.paths.DataDir, conf.tag)
	return nil
}

func ConfigFileExists(config Config) bool {
	conf := config.(*configInternal)
	_, err := os.Lstat(conf.configFilePath)
	return err == nil
}

var (
	MachineJobFromParams = machineJobFromParams
)

func EmptyConfig() Config {
	return &configInternal{}
}
