// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
)

func Password(config Config) string {
	c := config.(*configInternal)
	if c.stateDetails == nil {
		return c.apiDetails.password
	} else {
		return c.stateDetails.password
	}
	return ""
}

func WriteNewPassword(cfg Config) (string, error) {
	return cfg.(*configInternal).writeNewPassword()
}

func PatchConfig(c *gc.C, config Config, fieldName string, value interface{}) {
	conf := config.(*configInternal)
	switch fieldName {
	case "DataDir":
		conf.dataDir = value.(string)
	case "LogDir":
		conf.logDir = value.(string)
	case "Jobs":
		conf.jobs = value.([]params.MachineJob)[:]
	case "Tag":
		conf.tag = value.(string)
	case "Nonce":
		conf.nonce = value.(string)
	case "Password":
		conf.oldPassword = value.(string)
	case "CACert":
		conf.caCert = value.([]byte)[:]
	case "StateAddresses":
		addrs := value.([]string)[:]
		if conf.stateDetails == nil {
			conf.stateDetails = &connectionDetails{}
		}
		conf.stateDetails.addresses = addrs
	case "StatePassword":
		if conf.stateDetails == nil {
			conf.stateDetails = &connectionDetails{}
		}
		conf.stateDetails.password = value.(string)
	case "APIAddresses":
		addrs := value.([]string)[:]
		if conf.apiDetails == nil {
			conf.apiDetails = &connectionDetails{}
		}
		conf.apiDetails.addresses = addrs
	case "APIPassword":
		if conf.apiDetails == nil {
			conf.apiDetails = &connectionDetails{}
		}
		conf.apiDetails.password = value.(string)
	case "Values":
		conf.values = make(map[string]string)
		for k, v := range value.(map[string]string) {
			conf.values[k] = v
		}
	default:
		c.Fatalf("unknown field %q", fieldName)
	}
	conf.configFilePath = ConfigPath(conf.dataDir, conf.tag)
}

func ConfigFileExists(config Config) bool {
	conf := config.(*configInternal)
	_, err := os.Lstat(conf.configFilePath)
	return err == nil
}
