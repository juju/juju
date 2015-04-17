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
	case "DataDir":
		conf.dataDir = value.(string)
	case "LogDir":
		conf.logDir = value.(string)
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
	conf.configFilePath = ConfigPath(conf.dataDir, conf.tag)
	return nil
}

func ConfigFileExists(config Config) bool {
	conf := config.(*configInternal)
	_, err := os.Lstat(conf.configFilePath)
	return err == nil
}

var (
	MachineJobFromParams = machineJobFromParams
	IsLocalEnv           = &isLocalEnv
)
