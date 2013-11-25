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

// tagOffset represents the substring start value for the tag to return
// the logfileName value from the syslogtag.  Substrings in syslog are
// indexed from 1, hence the + 1.
const tagOffset = len("juju-") + 1

// The rsyslog conf for state server nodes.
// Messages are gathered from other nodes and accumulated in an all-machines.log file.
const stateServerRsyslogTemplate = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju{{namespace}}-{{logfileName}}:
$InputFileStateFile {{logfileName}}{{namespace}}
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun {{portNumber}}

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{namespace}},"%syslogtag:{{tagStart}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

:syslogtag, startswith, "juju{{namespace}}-" {{logDir}}/all-machines.log;JujuLogFormat{{namespace}}
& ~
`

// The rsyslog conf for non-state server nodes.
// Messages are forwarded to the state server node.
const nodeRsyslogTemplate = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju{{namespace}}-{{logfileName}}:
$InputFileStateFile {{logfileName}}{{namespace}}
$InputRunFileMonitor

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

:syslogtag, startswith, "juju{{namespace}}-" @{{bootstrapIP}}:{{portNumber}};LongTagForwardFormat
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
	// the name of the log file to tail.
	LogFileName string
	// the addresses of the state server to which messages should be forwarded.
	StateServerAddresses []string
	// the port number for the udp listener
	Port int
	// the directory for the logfiles
	LogDir string
	// namespace is used when there are multiple environments on one machine
	Namespace string
}

// NewForwardConfig creates a SyslogConfig instance used on unit nodes to forward log entries
// to the state server nodes.
func NewForwardConfig(logFile string, port int, namespace string, stateServerAddresses []string) *SyslogConfig {
	conf := &SyslogConfig{
		configTemplate:       nodeRsyslogTemplate,
		StateServerAddresses: stateServerAddresses,
		LogFileName:          logFile,
		Port:                 port,
		LogDir:               "/var/log/juju",
	}
	if namespace != "" {
		conf.Namespace = "-" + namespace
	}
	return conf
}

// NewAccumulateConfig creates a SyslogConfig instance used to accumulate log entries from the
// various unit nodes.
func NewAccumulateConfig(logFile string, port int, namespace string) *SyslogConfig {
	conf := &SyslogConfig{
		configTemplate: stateServerRsyslogTemplate,
		LogFileName:    logFile,
		Port:           port,
		LogDir:         "/var/log/juju",
	}
	if namespace != "" {
		conf.Namespace = "-" + namespace
	}
	return conf
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

	// TODO: for HA, we will want to send to all state server addresses (maybe).
	var bootstrapIP = func() string {
		addr := slConfig.StateServerAddresses[0]
		parts := strings.Split(addr, ":")
		return parts[0]
	}

	var logFilePath = func() string {
		return fmt.Sprintf("%s/%s.log", slConfig.LogDir, slConfig.LogFileName)
	}

	t := template.New("")
	t.Funcs(template.FuncMap{
		"logfileName": func() string { return slConfig.LogFileName },
		"bootstrapIP": bootstrapIP,
		"logfilePath": logFilePath,
		"portNumber":  func() int { return slConfig.Port },
		"logDir":      func() string { return slConfig.LogDir },
		"namespace":   func() string { return slConfig.Namespace },
		"tagStart":    func() int { return tagOffset + len(slConfig.Namespace) },
	})

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
