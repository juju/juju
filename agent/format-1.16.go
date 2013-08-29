// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path"

	"launchpad.net/goyaml"
)

const format116 = "format 1.16"

// formatter116 is the formatter for the 1.14 format.
type formatter116 struct {
}

// format116Serialization holds information for a given agent.
type format116Serialization struct {
	Tag   string
	Nonce string
	// CACert is base64 encoded
	CACert         string
	StateAddresses []string `yaml:",omitempty"`
	StatePassword  string   `yaml:",omitempty"`

	APIAddresses []string `yaml:",omitempty"`
	APIPassword  string   `yaml:",omitempty"`

	OldPassword string

	// Only state server machiens have this config.
	StateServerCert string `yaml:",omitempty"`
	StateServerKey  string `yaml:",omitempty"`
	APIPort         int    `yaml:",omitempty"`
}

// Ensure that the formatter116 struct implements the formatter interface.
var _ formatter = (*formatter116)(nil)

func (*formatter116) configFile(dirName string) string {
	return path.Join(dirName, "agent.conf")
}

func (formatter *formatter116) read(dirName string) (*configInternal, error) {
	data, err := ioutil.ReadFile(formatter.configFile(dirName))
	if err != nil {
		return nil, err
	}
	var format format116Serialization
	if err := goyaml.Unmarshal(data, &format); err != nil {
		return nil, err
	}
	caCert, err := base64.StdEncoding.DecodeString(format.CACert)
	if err != nil {
		return nil, err
	}
	stateServerCert, err := base64.StdEncoding.DecodeString(format.StateServerCert)
	if err != nil {
		return nil, err
	}
	stateServerKey, err := base64.StdEncoding.DecodeString(format.StateServerKey)
	if err != nil {
		return nil, err
	}
	config := &configInternal{
		tag:             format.Tag,
		nonce:           format.Nonce,
		caCert:          caCert,
		oldPassword:     format.OldPassword,
		stateServerCert: stateServerCert,
		stateServerKey:  stateServerKey,
		apiPort:         format.APIPort,
	}
	if len(format.StateAddresses) > 0 {
		config.stateDetails = &connectionDetails{
			format.StateAddresses,
			format.StatePassword,
		}
	}
	if len(format.APIAddresses) > 0 {
		config.apiDetails = &connectionDetails{
			format.APIAddresses,
			format.APIPassword,
		}
	}
	return config, nil
}

func (formatter *formatter116) makeFormat(config *configInternal) *format116Serialization {
	format := &format116Serialization{
		Tag:             config.tag,
		Nonce:           config.nonce,
		CACert:          base64.StdEncoding.EncodeToString(config.caCert),
		OldPassword:     config.oldPassword,
		StateServerCert: base64.StdEncoding.EncodeToString(config.stateServerCert),
		StateServerKey:  base64.StdEncoding.EncodeToString(config.stateServerKey),
		APIPort:         config.apiPort,
	}
	if config.stateDetails != nil {
		format.StateAddresses = config.stateDetails.addresses
		format.StatePassword = config.stateDetails.password
	}
	if config.apiDetails != nil {
		format.APIAddresses = config.apiDetails.addresses
		format.APIPassword = config.apiDetails.password
	}
	return format
}

func (formatter *formatter116) write(dirName string, config *configInternal) error {
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return err
	}
	if err := writeFormatFile(dirName, format116); err != nil {
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

func (formatter *formatter116) writeCommands(dirName string, config *configInternal) ([]string, error) {
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	commands := writeCommandsForFormat(dirName, format116)
	commands = append(commands,
		writeFileCommands(formatter.configFile(dirName), string(data), 0600)...)
	return commands, nil
}
