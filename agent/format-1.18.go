// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

const (
	format_1_18   = "format 1.18"
	versionLine   = "# format version: 1.18"
	agentConfFile = "agent.conf"
)

// formatter_1_18 is the formatter for the 1.18 format.
type formatter_1_18 struct {
}

// format_1_18Serialization holds information for a given agent.
type format_1_18Serialization struct {
	Tag     string
	DataDir string
	LogDir  string
	Nonce   string
	Jobs    []string `yaml:",omitempty"`

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

// Ensure that the formatter_1_18 struct implements the formatter interface.
var _ formatter = (*formatter_1_18)(nil)

// decode64 makes sure that for an empty string we have a nil slice, not an
// empty slice, which is what the base64 DecodeString function returns.
func (*formatter_1_18) decode64(value string) (result []byte, err error) {
	if value != "" {
		result, err = base64.StdEncoding.DecodeString(value)
	}
	return
}

func (formatter *formatter_1_18) read(configFilePath string) (*configInternal, error) {
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	// The format version should be on the first line
	parts := strings.SplitN(string(data), "\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid agent config format in %q", configFilePath)
	}
	formatVersion, configContents := parts[0], []byte(parts[1])

	if formatVersion != versionLine {
		return nil, fmt.Errorf(
			"unexpected agent config header %q in %q (expected %q)",
			formatVersion,
			configFilePath,
			versionLine,
		)
	}
	var format format_1_18Serialization
	if err := goyaml.Unmarshal(configContents, &format); err != nil {
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
		dataDir:         format.DataDir,
		logDir:          format.LogDir,
		nonce:           format.Nonce,
		caCert:          caCert,
		oldPassword:     format.OldPassword,
		stateServerCert: stateServerCert,
		stateServerKey:  stateServerKey,
		apiPort:         format.APIPort,
		values:          format.Values,
	}
	for _, jobName := range format.Jobs {
		job, err := state.MachineJobFromParams(params.MachineJob(jobName))
		if err != nil {
			return nil, err
		}
		config.jobs = append(config.jobs, job)
	}
	if config.logDir == "" {
		config.logDir = DefaultLogDir
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

func (formatter *formatter_1_18) makeFormat(config *configInternal) *format_1_18Serialization {
	jobs := make([]string, len(config.jobs))
	for i, job := range config.jobs {
		jobs[i] = job.String()
	}
	format := &format_1_18Serialization{
		Tag:             config.tag,
		DataDir:         config.dataDir,
		LogDir:          config.logDir,
		Jobs:            jobs,
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

func (formatter *formatter_1_18) write(config *configInternal) error {
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return err
	}
	data = []byte(versionLine + "\n" + string(data))
	if err := os.MkdirAll(config.Dir(), 0755); err != nil {
		return err
	}
	newFile := ConfigPath(config.dataDir, config.tag) + "-new"
	if err := ioutil.WriteFile(newFile, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(newFile, config.File(agentConfFile)); err != nil {
		return err
	}
	return nil
}

func (formatter *formatter_1_18) writeCommands(config *configInternal) ([]string, error) {
	conf := formatter.makeFormat(config)
	data, err := goyaml.Marshal(conf)
	if err != nil {
		return nil, err
	}
	commands := []string{"mkdir -p " + utils.ShQuote(config.Dir())}
	commands = append(commands, writeFileCommands(config.File(agentConfFile), string(data), 0600)...)
	return commands, nil
}

func (*formatter_1_18) migrate(config *configInternal) {
	if config.logDir == "" {
		config.logDir = DefaultLogDir
	}
}
