// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
)

var format_2_0 = formatter_2_0{}

// formatter_2_0 is the formatter for the 2.0 format.
type formatter_2_0 struct {
}

// Ensure that the formatter_2_0 struct implements the formatter interface.
var _ formatter = formatter_2_0{}

// format_2_0Serialization holds information for a given agent.
type format_2_0Serialization struct {
	Tag               string
	DataDir           string
	LogDir            string
	MetricsSpoolDir   string
	Nonce             string
	Jobs              []multiwatcher.MachineJob `yaml:",omitempty"`
	UpgradedToVersion *version.Number           `yaml:"upgradedToVersion"`

	CACert         string
	StateAddresses []string `yaml:",omitempty"`
	StatePassword  string   `yaml:",omitempty"`

	Model        string   `yaml:",omitempty"`
	APIAddresses []string `yaml:",omitempty"`
	APIPassword  string   `yaml:",omitempty"`

	OldPassword string
	Values      map[string]string

	// Only controller machines have these next items set.
	ControllerCert string `yaml:",omitempty"`
	ControllerKey  string `yaml:",omitempty"`
	CAPrivateKey   string `yaml:",omitempty"`
	APIPort        int    `yaml:",omitempty"`
	StatePort      int    `yaml:",omitempty"`
	SharedSecret   string `yaml:",omitempty"`
	SystemIdentity string `yaml:",omitempty"`
	MongoVersion   string `yaml:",omitempty"`
}

func init() {
	registerFormat(format_2_0)
}

func (formatter_2_0) version() string {
	return "2.0"
}

func (formatter_2_0) unmarshal(data []byte) (*configInternal, error) {
	// NOTE: this needs to handle the absence of StatePort and get it from the
	// address
	var format format_2_0Serialization
	if err := goyaml.Unmarshal(data, &format); err != nil {
		return nil, err
	}
	tag, err := names.ParseTag(format.Tag)
	if err != nil {
		return nil, err
	}
	var modelTag names.ModelTag
	if format.Model != "" {
		modelTag, err = names.ParseModelTag(format.Model)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	config := &configInternal{
		tag: tag,
		paths: NewPathsWithDefaults(Paths{
			DataDir:         format.DataDir,
			LogDir:          format.LogDir,
			MetricsSpoolDir: format.MetricsSpoolDir,
		}),
		jobs:              format.Jobs,
		upgradedToVersion: *format.UpgradedToVersion,
		nonce:             format.Nonce,
		model:             modelTag,
		caCert:            format.CACert,
		oldPassword:       format.OldPassword,
		values:            format.Values,
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
	if len(format.ControllerKey) != 0 {
		config.servingInfo = &params.StateServingInfo{
			Cert:           format.ControllerCert,
			PrivateKey:     format.ControllerKey,
			CAPrivateKey:   format.CAPrivateKey,
			APIPort:        format.APIPort,
			StatePort:      format.StatePort,
			SharedSecret:   format.SharedSecret,
			SystemIdentity: format.SystemIdentity,
		}
		// If private key is not present, infer it from the ports in the state addresses.
		if config.servingInfo.StatePort == 0 {
			if len(format.StateAddresses) == 0 {
				return nil, fmt.Errorf("server key found but no state port")
			}

			_, portString, err := net.SplitHostPort(format.StateAddresses[0])
			if err != nil {
				return nil, err
			}
			statePort, err := strconv.Atoi(portString)
			if err != nil {
				return nil, err
			}
			config.servingInfo.StatePort = statePort
		}

	}
	// Mongo version is set, we might be running a version other than default.
	if format.MongoVersion != "" {
		config.mongoVersion = format.MongoVersion
	}
	return config, nil
}

func (formatter_2_0) marshal(config *configInternal) ([]byte, error) {
	var modelTag string
	if config.model.Id() != "" {
		modelTag = config.model.String()
	}
	format := &format_2_0Serialization{
		Tag:               config.tag.String(),
		DataDir:           config.paths.DataDir,
		LogDir:            config.paths.LogDir,
		MetricsSpoolDir:   config.paths.MetricsSpoolDir,
		Jobs:              config.jobs,
		UpgradedToVersion: &config.upgradedToVersion,
		Nonce:             config.nonce,
		Model:             modelTag,
		CACert:            string(config.caCert),
		OldPassword:       config.oldPassword,
		Values:            config.values,
	}
	if config.servingInfo != nil {
		format.ControllerCert = config.servingInfo.Cert
		format.ControllerKey = config.servingInfo.PrivateKey
		format.CAPrivateKey = config.servingInfo.CAPrivateKey
		format.APIPort = config.servingInfo.APIPort
		format.StatePort = config.servingInfo.StatePort
		format.SharedSecret = config.servingInfo.SharedSecret
		format.SystemIdentity = config.servingInfo.SystemIdentity
	}
	if config.stateDetails != nil {
		format.StateAddresses = config.stateDetails.addresses
		format.StatePassword = config.stateDetails.password
	}
	if config.apiDetails != nil {
		format.APIAddresses = config.apiDetails.addresses
		format.APIPassword = config.apiDetails.password
	}
	if config.mongoVersion != "" {
		format.MongoVersion = string(config.mongoVersion)
	}
	return goyaml.Marshal(format)
}
