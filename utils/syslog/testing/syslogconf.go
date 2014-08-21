// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"text/template"

	gc "launchpad.net/gocheck"
)

var expectedAccumulateSyslogConfTemplate = `
$ModLoad imuxsock
$ModLoad imfile

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{.Namespace}},"%syslogtag:{{.Offset}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"


# start: Forwarding rule for foo
$ActionQueueType LinkedList
$ActionQueueFileName {{.MachineTag}}{{.Namespace}}_0
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile /var/log/juju{{.Namespace}}/ca-cert.pem
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

:syslogtag, startswith, "juju{{.Namespace}}-" @@foo:{{.Port}};LongTagForwardFormat
# end: Forwarding rule for foo

:syslogtag, startswith, "juju{{.Namespace}}-" stop

$FileCreateMode 0600

# Maximum size for the log on this outchannel is 512MB
# The command to execute when an outchannel as reached its size limit cannot accept any arguments
# that is why we have created the helper script for executing logrotate.
$outchannel logRotation,/var/log/juju{{.Namespace}}/all-machines.log,512000000,/var/log/juju{{.Namespace}}/logrotate.run

$RuleSet remote
$FileCreateMode 0600
:syslogtag, startswith, "juju{{.Namespace}}-" :omfile:$logRotation;JujuLogFormat{{.Namespace}}
:syslogtag, startswith, "juju{{.Namespace}}-" stop
$FileCreateMode 0600

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju{{.Namespace}}/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor

$ModLoad imtcp
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile /var/log/juju{{.Namespace}}/ca-cert.pem
$DefaultNetstreamDriverCertFile /var/log/juju{{.Namespace}}/rsyslog-cert.pem
$DefaultNetstreamDriverKeyFile /var/log/juju{{.Namespace}}/rsyslog-key.pem
$InputTCPServerStreamDriverAuthMode anon
$InputTCPServerStreamDriverMode 1 # run driver in TLS-only mode

$InputTCPServerBindRuleset remote
$InputTCPServerRun {{.Port}}

# switch back to default ruleset for further rules
$RuleSet RSYSLOG_DefaultRuleset
`

type templateArgs struct {
	MachineTag  string
	LogDir      string
	Namespace   string
	BootstrapIP string
	Port        int
	Offset      int
}

// ExpectedAccumulateSyslogConf returns the expected content for a rsyslog file on a state server.
func ExpectedAccumulateSyslogConf(c *gc.C, machineTag, namespace string, port int) string {
	if namespace != "" {
		namespace = "-" + namespace
	}
	t := template.Must(template.New("").Parse(expectedAccumulateSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, templateArgs{
		MachineTag: machineTag,
		Namespace:  namespace,
		Offset:     len("juju-") + len(namespace) + 1,
		Port:       port,
	})
	c.Assert(err, gc.IsNil)
	return conf.String()
}

var expectedForwardSyslogConfTemplate = `
$ModLoad imuxsock
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{.LogDir}}/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor

# start: Forwarding rule for server
$ActionQueueType LinkedList
$ActionQueueFileName {{.MachineTag}}{{.Namespace}}_0
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{.LogDir}}/ca-cert.pem
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"
:syslogtag, startswith, "juju{{.Namespace}}-" @@{{.BootstrapIP}}:{{.Port}};LongTagForwardFormat
# end: Forwarding rule for server

& ~
`

// ExpectedForwardSyslogConf returns the expected content for a rsyslog file on a host machine.
func ExpectedForwardSyslogConf(c *gc.C, machineTag, logDir, namespace, bootstrapIP string, port int) string {
	if namespace != "" {
		namespace = "-" + namespace
	}
	t := template.Must(template.New("").Parse(expectedForwardSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, templateArgs{
		MachineTag:  machineTag,
		LogDir:      logDir,
		Namespace:   namespace,
		BootstrapIP: bootstrapIP,
		Port:        port,
	})
	c.Assert(err, gc.IsNil)
	return conf.String()
}
