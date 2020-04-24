// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
)

var format_2_0 = formatter_2_0{}

// formatter_2_0 is the formatter for the 2.0 format.
type formatter_2_0 struct {
}

// Ensure that the formatter_2_0 struct implements the formatter interface.
var _ formatter = formatter_2_0{}

// format_2_0Serialization holds information for a given agent.
type format_2_0Serialization struct {
	Tag               string             `yaml:"tag,omitempty"`
	DataDir           string             `yaml:"datadir,omitempty"`
	TransientDataDir  string             `yaml:"transient-datadir,omitempty"`
	LogDir            string             `yaml:"logdir,omitempty"`
	MetricsSpoolDir   string             `yaml:"metricsspooldir,omitempty"`
	Nonce             string             `yaml:"nonce,omitempty"`
	Jobs              []model.MachineJob `yaml:"jobs,omitempty"`
	UpgradedToVersion *version.Number    `yaml:"upgradedToVersion"`

	CACert         string   `yaml:"cacert,omitempty"`
	StateAddresses []string `yaml:"stateaddresses,omitempty"`
	StatePassword  string   `yaml:"statepassword,omitempty"`

	Controller   string   `yaml:"controller,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	APIAddresses []string `yaml:"apiaddresses,omitempty"`
	APIPassword  string   `yaml:"apipassword,omitempty"`

	OldPassword   string            `yaml:"oldpassword,omitempty"`
	LoggingConfig string            `yaml:"loggingconfig,omitempty"`
	Values        map[string]string `yaml:"values"`

	// Only controller machines have these next items set.
	ControllerCert     string `yaml:"controllercert,omitempty"`
	ControllerKey      string `yaml:"controllerkey,omitempty"`
	CAPrivateKey       string `yaml:"caprivatekey,omitempty"`
	APIPort            int    `yaml:"apiport,omitempty"`
	ControllerAPIPort  int    `yaml:"controllerapiport,omitempty"`
	StatePort          int    `yaml:"stateport,omitempty"`
	SharedSecret       string `yaml:"sharedsecret,omitempty"`
	SystemIdentity     string `yaml:"systemidentity,omitempty"`
	MongoVersion       string `yaml:"mongoversion,omitempty"`
	MongoMemoryProfile string `yaml:"mongomemoryprofile,omitempty"`
	JujuDBSnapChannel  string `yaml:"juju-db-snap-channel,omitempty"`
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
	controllerTag, err := names.ParseControllerTag(format.Controller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelTag, err := names.ParseModelTag(format.Model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config := &configInternal{
		tag: tag,
		paths: NewPathsWithDefaults(Paths{
			DataDir:          format.DataDir,
			TransientDataDir: format.TransientDataDir,
			LogDir:           format.LogDir,
			MetricsSpoolDir:  format.MetricsSpoolDir,
		}),
		jobs:              format.Jobs,
		upgradedToVersion: *format.UpgradedToVersion,
		nonce:             format.Nonce,
		controller:        controllerTag,
		model:             modelTag,
		caCert:            format.CACert,
		statePassword:     format.StatePassword,
		oldPassword:       format.OldPassword,
		loggingConfig:     format.LoggingConfig,
		values:            format.Values,
	}
	if len(format.APIAddresses) > 0 {
		config.apiDetails = &apiDetails{
			format.APIAddresses,
			format.APIPassword,
		}
	}
	if len(format.ControllerKey) != 0 {
		config.servingInfo = &controller.StateServingInfo{
			Cert:              format.ControllerCert,
			PrivateKey:        format.ControllerKey,
			CAPrivateKey:      format.CAPrivateKey,
			APIPort:           format.APIPort,
			ControllerAPIPort: format.ControllerAPIPort,
			StatePort:         format.StatePort,
			SharedSecret:      format.SharedSecret,
			SystemIdentity:    format.SystemIdentity,
		}
		// If private key is not present, infer it from the ports in the state addresses.
		if config.servingInfo.StatePort == 0 {
			if len(format.StateAddresses) == 0 {
				return nil, errors.New("server key found but no state port")
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

	// TODO (anastasiamac 2016-06-17) Mongo version must be set.
	// For scenarios where mongo version is not set, we should still
	// be explicit about what "default" mongo version is. After these lines,
	// config.mongoVersion should not be empty under any circumstance.
	// Bug# 1593855
	if format.MongoVersion != "" {
		// Mongo version is set, we might be running a version other than default.
		config.mongoVersion = format.MongoVersion
	}
	if format.MongoMemoryProfile != "" {
		config.mongoMemoryProfile = format.MongoMemoryProfile
	}
	if format.JujuDBSnapChannel != "" {
		config.jujuDBSnapChannel = format.JujuDBSnapChannel
	}
	return config, nil
}

func (formatter_2_0) marshal(config *configInternal) ([]byte, error) {
	controllerTag := config.controller.String()
	modelTag := config.model.String()
	format := &format_2_0Serialization{
		Tag:               config.tag.String(),
		DataDir:           config.paths.DataDir,
		TransientDataDir:  config.paths.TransientDataDir,
		LogDir:            config.paths.LogDir,
		MetricsSpoolDir:   config.paths.MetricsSpoolDir,
		Jobs:              config.jobs,
		UpgradedToVersion: &config.upgradedToVersion,
		Nonce:             config.nonce,
		Controller:        controllerTag,
		Model:             modelTag,
		CACert:            config.caCert,
		OldPassword:       config.oldPassword,
		LoggingConfig:     config.loggingConfig,
		Values:            config.values,
	}
	if config.servingInfo != nil {
		format.ControllerCert = config.servingInfo.Cert
		format.ControllerKey = config.servingInfo.PrivateKey
		format.CAPrivateKey = config.servingInfo.CAPrivateKey
		format.APIPort = config.servingInfo.APIPort
		format.ControllerAPIPort = config.servingInfo.ControllerAPIPort
		format.StatePort = config.servingInfo.StatePort
		format.SharedSecret = config.servingInfo.SharedSecret
		format.SystemIdentity = config.servingInfo.SystemIdentity
		format.StatePassword = config.statePassword
	}
	if config.apiDetails != nil {
		format.APIAddresses = config.apiDetails.addresses
		format.APIPassword = config.apiDetails.password
	}
	if config.mongoVersion != "" {
		format.MongoVersion = config.mongoVersion
	}
	if config.mongoMemoryProfile != "" {
		format.MongoMemoryProfile = config.mongoMemoryProfile
	}
	if config.jujuDBSnapChannel != "" {
		format.JujuDBSnapChannel = config.jujuDBSnapChannel
	}
	return goyaml.Marshal(format)
}
