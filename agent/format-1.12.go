// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

const format_1_12 = "format 1.12"

// formatter_1_12 is the formatter for the 1.12 format.
type formatter_1_12 struct {
}

// format_1_12Serialization holds information stored in the agent.conf file.
type format_1_12Serialization struct {
	// StateServerCert and StateServerKey hold the state server
	// certificate and private key in PEM format.
	StateServerCert []byte `yaml:",omitempty"`
	StateServerKey  []byte `yaml:",omitempty"`

	StatePort int `yaml:",omitempty"`
	APIPort   int `yaml:",omitempty"`

	// OldPassword specifies a password that should be
	// used to connect to the state if StateInfo.Password
	// is blank or invalid.
	OldPassword string

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// StateInfo specifies how the agent should connect to the
	// state.  The password may be empty if an old password is
	// specified, or when bootstrapping.
	StateInfo *state.Info `yaml:",omitempty"`

	// OldAPIPassword specifies a password that should
	// be used to connect to the API if APIInfo.Password
	// is blank or invalid.
	OldAPIPassword string

	// APIInfo specifies how the agent should connect to the
	// state through the API.
	APIInfo *api.Info `yaml:",omitempty"`
}

// Ensure that the formatter_1_12 struct implements the formatter interface.
var _ formatter = (*formatter_1_12)(nil)

func (*formatter_1_12) configFile(dirName string) string {
	return path.Join(dirName, "agent.conf")
}

func (formatter *formatter_1_12) read(dirName string) (*configInternal, error) {
	data, err := ioutil.ReadFile(formatter.configFile(dirName))
	if err != nil {
		return nil, err
	}
	var conf format_1_12Serialization
	if err := goyaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	var stateDetails *connectionDetails
	var caCert []byte
	var tag string
	if conf.StateInfo != nil {
		stateDetails = &connectionDetails{
			conf.StateInfo.Addrs,
			conf.StateInfo.Password,
		}
		tag = conf.StateInfo.Tag
		caCert = conf.StateInfo.CACert
	}
	var apiDetails *connectionDetails
	if conf.APIInfo != nil {
		apiDetails = &connectionDetails{
			conf.APIInfo.Addrs,
			conf.APIInfo.Password,
		}
		tag = conf.APIInfo.Tag
		caCert = conf.APIInfo.CACert
	}
	return &configInternal{
		tag:             tag,
		nonce:           conf.MachineNonce,
		caCert:          caCert,
		stateDetails:    stateDetails,
		apiDetails:      apiDetails,
		oldPassword:     conf.OldPassword,
		stateServerCert: conf.StateServerCert,
		stateServerKey:  conf.StateServerKey,
		apiPort:         conf.APIPort,
		values:          map[string]string{},
	}, nil
}

func (formatter *formatter_1_12) makeAgentConf(config *configInternal) *format_1_12Serialization {
	format := &format_1_12Serialization{
		StateServerCert: config.stateServerCert,
		StateServerKey:  config.stateServerKey,
		APIPort:         config.apiPort,
		OldPassword:     config.oldPassword,
		MachineNonce:    config.nonce,
	}
	if config.stateDetails != nil {
		// It is fine that we are copying the slices for the addresses.
		format.StateInfo = &state.Info{
			Addrs:    config.stateDetails.addresses,
			Password: config.stateDetails.password,
			Tag:      config.tag,
			CACert:   config.caCert,
		}
	}
	if config.apiDetails != nil {
		format.APIInfo = &api.Info{
			Addrs:    config.apiDetails.addresses,
			Password: config.apiDetails.password,
			Tag:      config.tag,
			CACert:   config.caCert,
		}
	}
	return format
}

func (formatter *formatter_1_12) write(config *configInternal) error {
	dirName := config.Dir()
	conf := formatter.makeAgentConf(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirName, 0755); err != nil {
		return err
	}
	newFile := path.Join(dirName, "agent.conf-new")
	if err := ioutil.WriteFile(newFile, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(newFile, formatter.configFile(dirName)); err != nil {
		return err
	}
	return nil
}

func (formatter *formatter_1_12) writeCommands(config *configInternal) ([]string, error) {
	dirName := config.Dir()
	conf := formatter.makeAgentConf(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	var commands []string
	addCommand := func(f string, a ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, a...))
	}
	filename := utils.ShQuote(formatter.configFile(dirName))
	addCommand("mkdir -p %s", utils.ShQuote(dirName))
	addCommand("install -m %o /dev/null %s", 0600, filename)
	addCommand(`printf '%%s\n' %s > %s`, utils.ShQuote(string(data)), filename)
	return commands, nil
}

func (*formatter_1_12) migrate(config *configInternal) {
}
