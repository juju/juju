// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/juju/osenv"
)

const (
	format_1_16 = "format 1.16"
	// Old environment variables that are now stored in agent config.
	JujuLxcBridge         = "JUJU_LXC_BRIDGE"
	JujuProviderType      = "JUJU_PROVIDER_TYPE"
	JujuStorageDir        = "JUJU_STORAGE_DIR"
	JujuStorageAddr       = "JUJU_STORAGE_ADDR"
	JujuSharedStorageDir  = "JUJU_SHARED_STORAGE_DIR"
	JujuSharedStorageAddr = "JUJU_SHARED_STORAGE_ADDR"
)

// formatter_1_16 is the formatter for the 1.16 format.
type formatter_1_16 struct {
}

// format_1_16Serialization holds information for a given agent.
type format_1_16Serialization struct {
	Tag   string
	Nonce string
	// CACert is base64 encoded
	CACert         string
	StateAddresses []string `yaml:",omitempty"`
	StatePassword  string   `yaml:",omitempty"`

	APIAddresses []string `yaml:",omitempty"`
	APIPassword  string   `yaml:",omitempty"`

	OldPassword string
	Values      map[string]string

	// Only state server machines have these next three items
	StateServerCert string `yaml:",omitempty"`
	StateServerKey  string `yaml:",omitempty"`
	APIPort         int    `yaml:",omitempty"`
}

// Ensure that the formatter_1_16 struct implements the formatter interface.
var _ formatter = (*formatter_1_16)(nil)

func (*formatter_1_16) configFile(dirName string) string {
	return path.Join(dirName, "agent.conf")
}

// decode64 makes sure that for an empty string we have a nil slice, not an
// empty slice, which is what the base64 DecodeString function returns.
func (*formatter_1_16) decode64(value string) (result []byte, err error) {
	if value != "" {
		result, err = base64.StdEncoding.DecodeString(value)
	}
	return
}

func (formatter *formatter_1_16) read(dirName string) (*configInternal, error) {
	data, err := ioutil.ReadFile(formatter.configFile(dirName))
	if err != nil {
		return nil, err
	}
	var format format_1_16Serialization
	if err := goyaml.Unmarshal(data, &format); err != nil {
		return nil, err
	}
	caCert, err := formatter.decode64(format.CACert)
	if err != nil {
		return nil, err
	}
	stateServerCert, err := formatter.decode64(format.StateServerCert)
	if err != nil {
		return nil, err
	}
	stateServerKey, err := formatter.decode64(format.StateServerKey)
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
		values:          format.Values,
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

func (formatter *formatter_1_16) makeFormat(config *configInternal) *format_1_16Serialization {
	format := &format_1_16Serialization{
		Tag:             config.tag,
		Nonce:           config.nonce,
		CACert:          base64.StdEncoding.EncodeToString(config.caCert),
		OldPassword:     config.oldPassword,
		StateServerCert: base64.StdEncoding.EncodeToString(config.stateServerCert),
		StateServerKey:  base64.StdEncoding.EncodeToString(config.stateServerKey),
		APIPort:         config.apiPort,
		Values:          config.values,
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

func (formatter *formatter_1_16) write(config *configInternal) error {
	dirName := config.Dir()
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return err
	}
	// Writing the format file makes sure that dirName exists.  We should
	// really be writing the format and new config files into a separate
	// directory, and renaming the directory, and moving the old agend
	// directory to ".old".
	if err := writeFormatFile(dirName, format_1_16); err != nil {
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

func (formatter *formatter_1_16) writeCommands(config *configInternal) ([]string, error) {
	dirName := config.Dir()
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	commands := writeCommandsForFormat(dirName, format_1_16)
	commands = append(commands,
		writeFileCommands(formatter.configFile(dirName), string(data), 0600)...)
	return commands, nil
}

func (*formatter_1_16) migrate(config *configInternal) {
	for _, name := range []struct {
		environment string
		config      string
	}{{
		JujuProviderType,
		ProviderType,
	}, {
		osenv.JujuContainerType,
		ContainerType,
	}, {
		JujuLxcBridge,
		LxcBridge,
	}, {
		JujuStorageDir,
		StorageDir,
	}, {
		JujuStorageAddr,
		StorageAddr,
	}, {
		JujuSharedStorageDir,
		SharedStorageDir,
	}, {
		JujuSharedStorageAddr,
		SharedStorageAddr,
	}} {
		value := os.Getenv(name.environment)
		if value != "" {
			config.values[name.config] = value
		}
	}
}
