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

// formatter112 is the formatter for the 1.12 format.
type formatter112 struct {
}

// agentConf holds information stored in the agent.conf file.
type agentConf struct {
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

// Ensure that the formatter112 struct implements the formatter interface.
var _ formatter = (*formatter112)(nil)

func (*formatter112) configFile(dirName string) string {
	return path.Join(dirName, "agent.conf")
}

func (formatter *formatter112) read(dirName string) (*configInternal, error) {
	data, err := ioutil.ReadFile(formatter.configFile(dirName))
	if err != nil {
		return nil, err
	}
	var conf agentConf
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
	}, nil
}

func (formatter *formatter112) makeAgentConf(config *configInternal) *agentConf {
	var stateInfo *state.Info
	var apiInfo *api.Info
	if config.stateDetails != nil {
		// It is fine that we are copying the slices for the addresses.
		stateInfo = &state.Info{
			Addrs:    config.stateDetails.addresses,
			Password: config.stateDetails.password,
			Tag:      config.tag,
			CACert:   config.caCert,
		}
	}
	if config.apiDetails != nil {
		apiInfo = &api.Info{
			Addrs:    config.apiDetails.addresses,
			Password: config.apiDetails.password,
			Tag:      config.tag,
			CACert:   config.caCert,
		}
	}
	return &agentConf{
		StateServerCert: config.stateServerCert,
		StateServerKey:  config.stateServerKey,
		APIPort:         config.apiPort,
		OldPassword:     config.oldPassword,
		MachineNonce:    config.nonce,
		StateInfo:       stateInfo,
		APIInfo:         apiInfo,
	}
}

func (formatter *formatter112) write(dirName string, config *configInternal) error {
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

func (formatter *formatter112) writeCommands(dirName string, config *configInternal) ([]string, error) {
	conf := formatter.makeAgentConf(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	var commands []string
	addCommand := func(f string, a ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, a...))
	}
	f := utils.ShQuote(formatter.configFile(dirName))
	addCommand("mkdir -p %s", utils.ShQuote(dirName))
	addCommand("install -m %o /dev/null %s", 0600, f)
	addCommand(`printf '%%s\n' %s > %s`, utils.ShQuote(string(data)), f)
	return commands, nil
}
