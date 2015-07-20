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
// in the local provider log dir to point to that file. By
// default rsyslog creates files with 0644, but in the ubuntu package, the
// setting is changed to 0640. Using a new action directive (new as in
// not-legacy), we can specify the file create mode so it doesn't use
// the default.
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
$ModLoad imuxsock
$ModLoad imfile

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{namespace}},"%syslogtag:{{tagStart}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

{{range $i, $stateServerIP := stateServerHosts}}
# start: Forwarding rule for {{$stateServerIP}}
$ActionQueueType LinkedList
$ActionQueueFileName {{logfileName}}{{namespace}}_{{$i}}
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$ActionQueueMaxDiskSpace 512M
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{tlsCACertPath}}
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

:syslogtag, startswith, "juju{{namespace}}-" @@{{$stateServerIP}}:{{portNumber}};LongTagForwardFormat
# end: Forwarding rule for {{$stateServerIP}}
{{end}}
:syslogtag, startswith, "juju{{namespace}}-" stop

$FileCreateMode 0600

# Maximum size for the log on this outchannel is 512MB
# The command to execute when an outchannel as reached its size limit cannot accept any arguments
# that is why we have created the helper script for executing logrotate.
$outchannel logRotation,{{logDir}}/all-machines.log,536870912,{{logrotateHelperPath}}

$RuleSet remote
$FileCreateMode 0600
:syslogtag, startswith, "juju{{namespace}}-" :omfile:$logRotation;JujuLogFormat{{namespace}}
:syslogtag, startswith, "juju{{namespace}}-" stop
$FileCreateMode 0600

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
$InputTCPMaxSessions 10000 # default is 200, all agents connect to all rsyslog daemons

$InputTCPServerBindRuleset remote
$InputTCPServerRun {{portNumber}}

# switch back to default ruleset for further rules
$RuleSet RSYSLOG_DefaultRuleset
`

// The rsyslog conf for non-state server nodes.
// Messages are forwarded to the state server node.
//
// Each forwarding rule must be repeated in full for every state server and
// each rule must use a unique ActionQueueFileName
// See: http://www.rsyslog.com/doc/rsyslog_reliable_forwarding.html
const nodeRsyslogTemplate = `
$ModLoad imuxsock
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{logfilePath}}
$InputFileTag juju{{namespace}}-{{logfileName}}:
$InputFileStateFile {{logfileName}}{{namespace}}
$InputRunFileMonitor
{{range $i, $stateServerIP := stateServerHosts}}
# start: Forwarding rule for {{$stateServerIP}}
$ActionQueueType LinkedList
$ActionQueueFileName {{logfileName}}{{namespace}}_{{$i}}
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$ActionQueueMaxDiskSpace 512M
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{tlsCACertPath}}
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"
:syslogtag, startswith, "juju{{namespace}}-" @@{{$stateServerIP}}:{{portNumber}};LongTagForwardFormat
# end: Forwarding rule for {{$stateServerIP}}
{{end}}
& ~
`

// The logrotate conf for state serve nodes.
// default size is 512MB, ensuring that the log + one rotation
// will never take up more than 1GB of space.
//
// The size of the logrotate configuration is low for
// a very specific reason, see config comment.
const logrotateConf = `
{{.LogDir}}/all-machines.log {
    # rsyslogd informs logrotate when to rotate.
    # The size specified here must be less than or equal to the log size
    # when rsyslogd informs logrotate, or logrotate will take no action.
    # The size value is otherwise unimportant.
    size 1K
    # maximum of one old file
    rotate 1
    # counting old files starts at 1 rather than 0
    start 1
	# ensure new file is created with the correct permissions
    create 600
	# reload rsyslog after rotation so it will use the new file
    postrotate
      service rsyslog reload
    endscript
}
`

var logrotateConfTemplate = template.Must(template.New("logrotate.conf").Parse(logrotateConf))

// The logrotate helper script for state server nodes.
// We specify a state file to ensure we have the proper permissions.
const logrotateHelper = `
/usr/sbin/logrotate -s {{.LogDir}}/logrotate.state {{.LogrotateConfPath}}
`

var logrotateHelperTemplate = template.Must(template.New("logrotate.run").Parse(logrotateHelper))

// nodeRsyslogTemplateTLSHeader is prepended to
// nodeRsyslogTemplate if TLS is to be used.
const nodeRsyslogTemplateTLSHeader = `
`

const (
	defaultConfigDir               = "/etc/rsyslog.d"
	defaultCACertFileName          = "ca-cert.pem"
	defaultServerCertFileName      = "rsyslog-cert.pem"
	defaultServerKeyFileName       = "rsyslog-key.pem"
	defaultLogrotateConfFileName   = "logrotate.conf"
	defaultLogrotateHelperFileName = "logrotate.run"
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
	// the name of the logrotate configuration file.
	LogrotateConfFileName string
	// the name of the script that executes the logrotate command.
	LogrotateHelperFileName string
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
	// directory where juju stores its config files
	JujuConfigDir string
}

// NewForwardConfig creates a SyslogConfig instance used on unit nodes to forward log entries
// to the state server nodes.
func NewForwardConfig(cfg *SyslogConfig) {
	cfg.configTemplate = nodeRsyslogTemplate
}

// NewAccumulateConfig creates a SyslogConfig instance used to accumulate log entries from the
// various unit nodes.
func NewAccumulateConfig(cfg *SyslogConfig) {
	cfg.configTemplate = stateServerRsyslogTemplate
}

func either(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (slConfig *SyslogConfig) renderNamespace() string {
	if slConfig.Namespace == "" {
		return ""
	}
	return "-" + slConfig.Namespace
}

func (slConfig *SyslogConfig) ConfigFilePath() string {
	dir := either(slConfig.ConfigDir, defaultConfigDir)
	return filepath.Join(dir, slConfig.ConfigFileName)
}

func (slConfig *SyslogConfig) CACertPath() string {
	filename := either(slConfig.CACertFileName, defaultCACertFileName)
	return filepath.Join(slConfig.JujuConfigDir, filename)
}

func (slConfig *SyslogConfig) ServerCertPath() string {
	filename := either(slConfig.ServerCertFileName, defaultServerCertFileName)
	return filepath.Join(slConfig.JujuConfigDir, filename)
}

func (slConfig *SyslogConfig) ServerKeyPath() string {
	filename := either(slConfig.ServerCertFileName, defaultServerKeyFileName)
	return filepath.Join(slConfig.JujuConfigDir, filename)
}

// LogrotateConfPath returns the the the entire logrotate.conf path including filename.
func (slConfig *SyslogConfig) LogrotateConfPath() string {
	filename := either(slConfig.LogrotateConfFileName, defaultLogrotateConfFileName)
	return filepath.Join(slConfig.JujuConfigDir, filename)
}

// LogrotateHelperPath returns the entire logrotate.helper path including filename.
func (slConfig *SyslogConfig) LogrotateHelperPath() string {
	filename := either(slConfig.LogrotateHelperFileName, defaultLogrotateHelperFileName)
	return filepath.Join(slConfig.JujuConfigDir, filename)
}

// LogrotateConfFile returns a ready to write to disk byte array of the logrotate.conf file.
func (slConfig *SyslogConfig) LogrotateConfFile() ([]byte, error) {
	return slConfig.logrotateRender(logrotateConfTemplate)
}

// LogrotateHelperFile returns a ready to write to disk byte array of the logrotate.helper file.
func (slConfig *SyslogConfig) LogrotateHelperFile() ([]byte, error) {
	return slConfig.logrotateRender(logrotateHelperTemplate)
}

func (slConfig *SyslogConfig) logrotateRender(t *template.Template) ([]byte, error) {
	var buffer bytes.Buffer
	if err := t.Execute(&buffer, slConfig); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Render generates the rsyslog config.
func (slConfig *SyslogConfig) Render() ([]byte, error) {
	var stateServerHosts = func() []string {
		var hosts []string
		for _, addr := range slConfig.StateServerAddresses {
			parts := strings.Split(addr, ":")
			hosts = append(hosts, parts[0])
		}
		return hosts
	}

	var logFilePath = func() string {
		return fmt.Sprintf("%s/%s.log", slConfig.LogDir, slConfig.LogFileName)
	}

	t := template.New("syslogConfig")
	t.Funcs(template.FuncMap{
		"logfileName":         func() string { return slConfig.LogFileName },
		"stateServerHosts":    stateServerHosts,
		"logfilePath":         logFilePath,
		"portNumber":          func() int { return slConfig.Port },
		"logDir":              func() string { return slConfig.LogDir },
		"namespace":           slConfig.renderNamespace,
		"tagStart":            func() int { return tagOffset + len(slConfig.renderNamespace()) },
		"tlsCACertPath":       slConfig.CACertPath,
		"tlsCertPath":         slConfig.ServerCertPath,
		"tlsKeyPath":          slConfig.ServerKeyPath,
		"logrotateHelperPath": slConfig.LogrotateHelperPath,
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
