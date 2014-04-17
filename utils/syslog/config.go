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
//
// The apparmor profile is quite strict about where rsyslog can write files.
// Instead of poking with the profile, the local provider now logs to
// {{logDir}}-{{user}}-{{env name}}/all-machines.log, and a symlink is made
// in the local provider log dir to point to that file. The file is also
// created with 0644 so the user can read it without poking permissions. By
// default rsyslog creates files with 0644, but in the ubuntu package, the
// setting is changed to 0640, which means normal users can't read the log
// file. Using a new action directive (new as in not-legacy), we can specify
// the file create mode so it doesn't use the default.
//
// I would dearly love to write the filtering action as follows to avoid setting
// and resetting the global $FileCreateMode, but alas, precise doesn't support it
//
// if $syslogtag startswith "juju{{namespace}}-" then
//   action(type="omfile"
//          File="{{logDir}}{{namespace}}/all-machines.log"
//          Template="JujuLogFormat{{namespace}}"
//          FileCreateMode="0644")
// & stop
//
// Instead we need to mess with the global FileCreateMode.  We set it back
// to the ubuntu default after defining our rule.
const stateServerRsyslogTemplate = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju{{namespace}}-{{logfileName}}:
$InputFileStateFile {{logfileName}}{{namespace}}
$InputRunFileMonitor

$ModLoad imtcp
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{tlsCACertPath}}
$DefaultNetstreamDriverCertFile {{tlsCertPath}}
$DefaultNetstreamDriverKeyFile {{tlsKeyPath}}
$InputTCPServerStreamDriverAuthMode anon
$InputTCPServerStreamDriverMode 1 # run driver in TLS-only mode
$InputTCPServerRun {{portNumber}}

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{namespace}},"%syslogtag:{{tagStart}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$FileCreateMode 0644
:syslogtag, startswith, "juju{{namespace}}-" {{logDir}}/all-machines.log;JujuLogFormat{{namespace}}
& ~
$FileCreateMode 0640
`

// The rsyslog conf for non-state server nodes.
// Messages are forwarded to the state server node.
const nodeRsyslogTemplate = `
$ModLoad imfile

# Enable reliable forwarding.
$ActionQueueType LinkedList
$ActionQueueFileName {{logfileName}}{{namespace}}
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju{{namespace}}-{{logfileName}}:
$InputFileStateFile {{logfileName}}{{namespace}}
$InputRunFileMonitor

$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{tlsCACertPath}}
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

:syslogtag, startswith, "juju{{namespace}}-" @@{{bootstrapIP}}:{{portNumber}};LongTagForwardFormat
& ~
`

// nodeRsyslogTemplateTLSHeader is prepended to
// nodeRsyslogTemplate if TLS is to be used.
const nodeRsyslogTemplateTLSHeader = `
`

const (
	defaultConfigDir          = "/etc/rsyslog.d"
	defaultCACertFileName     = "ca-cert.pem"
	defaultServerCertFileName = "rsyslog-cert.pem"
	defaultServerKeyFileName  = "rsyslog-key.pem"
)

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
	// CA certificate file name.
	CACertFileName string
	// Server certificate file name.
	ServerCertFileName string
	// Server private key file name.
	ServerKeyFileName string
	// the port number for the listener
	Port int
	// the directory for the logfiles
	LogDir string
	// namespace is used when there are multiple environments on one machine
	Namespace string
}

// NewForwardConfig creates a SyslogConfig instance used on unit nodes to forward log entries
// to the state server nodes.
func NewForwardConfig(logFile, logDir string, port int, namespace string, stateServerAddresses []string) *SyslogConfig {
	conf := &SyslogConfig{
		configTemplate:       nodeRsyslogTemplate,
		StateServerAddresses: stateServerAddresses,
		LogFileName:          logFile,
		Port:                 port,
		LogDir:               logDir,
	}
	if namespace != "" {
		conf.Namespace = "-" + namespace
	}
	return conf
}

// NewAccumulateConfig creates a SyslogConfig instance used to accumulate log entries from the
// various unit nodes.
func NewAccumulateConfig(logFile, logDir string, port int, namespace string) *SyslogConfig {
	conf := &SyslogConfig{
		configTemplate: stateServerRsyslogTemplate,
		LogFileName:    logFile,
		Port:           port,
		LogDir:         logDir,
	}
	if namespace != "" {
		conf.Namespace = "-" + namespace
	}
	return conf
}

func either(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (slConfig *SyslogConfig) ConfigFilePath() string {
	dir := either(slConfig.ConfigDir, defaultConfigDir)
	return filepath.Join(dir, slConfig.ConfigFileName)
}

func (slConfig *SyslogConfig) CACertPath() string {
	filename := either(slConfig.CACertFileName, defaultCACertFileName)
	return filepath.Join(slConfig.LogDir, filename)
}

func (slConfig *SyslogConfig) ServerCertPath() string {
	filename := either(slConfig.ServerCertFileName, defaultServerCertFileName)
	return filepath.Join(slConfig.LogDir, filename)
}

func (slConfig *SyslogConfig) ServerKeyPath() string {
	filename := either(slConfig.ServerCertFileName, defaultServerKeyFileName)
	return filepath.Join(slConfig.LogDir, filename)
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
		"logfileName":   func() string { return slConfig.LogFileName },
		"bootstrapIP":   bootstrapIP,
		"logfilePath":   logFilePath,
		"portNumber":    func() int { return slConfig.Port },
		"logDir":        func() string { return slConfig.LogDir },
		"namespace":     func() string { return slConfig.Namespace },
		"tagStart":      func() int { return tagOffset + len(slConfig.Namespace) },
		"tlsCACertPath": slConfig.CACertPath,
		"tlsCertPath":   slConfig.ServerCertPath,
		"tlsKeyPath":    slConfig.ServerKeyPath,
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
