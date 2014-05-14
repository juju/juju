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

$RuleSet local

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

& ~
$FileCreateMode 0640

$RuleSet remote
$FileCreateMode 0644
:syslogtag, startswith, "juju{{.Namespace}}-" /var/log/juju{{.Namespace}}/all-machines.log;JujuLogFormat{{.Namespace}}
& ~
$FileCreateMode 0640

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju{{.Namespace}}/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor
$DefaultRuleset local

$ModLoad imtcp
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile /var/log/juju{{.Namespace}}/ca-cert.pem
$DefaultNetstreamDriverCertFile /var/log/juju{{.Namespace}}/rsyslog-cert.pem
$DefaultNetstreamDriverKeyFile /var/log/juju{{.Namespace}}/rsyslog-key.pem
$InputTCPServerStreamDriverAuthMode anon
$InputTCPServerStreamDriverMode 1 # run driver in TLS-only mode

$InputTCPServerBindRuleset remote
$InputTCPServerRun {{.Port}}
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
