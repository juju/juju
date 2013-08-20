// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"text/template"
)

// The rsyslog conf for state server nodes.
// Messages are gathered from other nodes and accumulated in an all-machines.log file.
const stateServerRsyslogTemplate = `
$ModLoad imfile

$InputFileStateFile {{statefilePath}}
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag local-juju-{{logfileName}}:
$InputFileStateFile {{logfileName}}
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun 514

# Messages received from remote rsyslog machines contain a leading space so we
# need to account for that.
$template JujuLogFormatLocal,"%HOSTNAME%:%msg:::drop-last-lf%\n"
$template JujuLogFormat,"%HOSTNAME%:%msg:2:2048:drop-last-lf%\n"

:syslogtag, startswith, "juju-" /var/log/juju/all-machines.log;JujuLogFormat
& ~
:syslogtag, startswith, "local-juju-" /var/log/juju/all-machines.log;JujuLogFormatLocal
& ~
`

// The rsyslog conf for non-state server nodes.
// Messages are forwarded to the state server node.
const nodeRsyslogTemplate = `
$ModLoad imfile

$InputFileStateFile {{statefilePath}}
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju-{{logfileName}}:
$InputFileStateFile {{logfileName}}
$InputRunFileMonitor

:syslogtag, startswith, "juju-" @{{bootstrapIP}}:514
& ~
`

const defaultConfigDir = "/etc/rsyslog.d"

// SyslogConfigRenderer instances are used to generate a rsyslog conf file.
type SyslogConfigRenderer interface {
	Render() ([]byte, error)
}

// SyslogConfig provides a means to configure and generate rsyslog conf files for
// the state server nodes and unit nodes.
// rsyslog is configured to tail the specified log file.
type SyslogConfig struct {
	// the template representing the config file contents.
	configTemplate string
	// the directory where the config file is written.
	ConfigDir string
	// the config file name.
	ConfigFileName string
	// the name of tghe log file to tail.
	LogFileName string
	// the addresses of the state server to which messages should be forwarded.
	StateServerAddresses []string
}

// NewForwardConfig creates a SyslogConfig instance used on unit nodes to forward log entries
// to the state server nodes.
func NewForwardConfig(logFile string, stateServerAddresses []string) *SyslogConfig {
	return &SyslogConfig{
		configTemplate:       nodeRsyslogTemplate,
		StateServerAddresses: stateServerAddresses,
		LogFileName:          logFile,
	}
}

// NewAccumulateConfig creates a SyslogConfig instance used to accumulate log entries from the
// various unit nodes.
func NewAccumulateConfig(logFile string) *SyslogConfig {
	return &SyslogConfig{
		configTemplate: stateServerRsyslogTemplate,
		LogFileName:    logFile,
	}
}

func (slConfig *SyslogConfig) ConfigFilePath() string {
	dir := slConfig.ConfigDir
	if dir == "" {
		dir = defaultConfigDir
	}
	return filepath.Join(dir, slConfig.ConfigFileName)
}

// Render generates the rsyslog config.
func (slConfig *SyslogConfig) Render() ([]byte, error) {

	var bootstrapIP = func() string {
		addr := slConfig.StateServerAddresses[0]
		parts := strings.Split(addr, ":")
		return parts[0]
	}

	var logFileName = func() string {
		return slConfig.LogFileName
	}

	var logFilePath = func() string {
		return fmt.Sprintf("/var/log/juju/%s.log", slConfig.LogFileName)
	}

	var stateFilePath = func() string {
		return fmt.Sprintf("/var/spool/rsyslog/juju-%s-state", slConfig.LogFileName)
	}

	t := template.New("")
	t.Funcs(template.FuncMap{"logfileName": logFileName})
	t.Funcs(template.FuncMap{"bootstrapIP": bootstrapIP})
	t.Funcs(template.FuncMap{"logfilePath": logFilePath})
	t.Funcs(template.FuncMap{"statefilePath": stateFilePath})

	// Process the rsyslog config template and echo to the conf file.
	p, err := t.Parse(slConfig.configTemplate)
	if err != nil {
		return nil, err
	}
	var confBuf bytes.Buffer
	if err := p.Execute(&confBuf, nil); err != nil {
		return nil, err
	}
	return confBuf.Bytes(), nil
}

// Write generates and writes the rsyslog config.
func (slConfig *SyslogConfig) Write() error {
	data, err := slConfig.Render()
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(slConfig.ConfigFilePath(), data, 0644)
	return err
}
