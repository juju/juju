// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"text/template"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var expectedAccumulateSyslogConfTemplate = `
$ModLoad imuxsock
$ModLoad imfile

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{.Namespace}},"%syslogtag:{{.Offset}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"


# start: Forwarding rule for {{.Server}}
$ActionQueueType LinkedList
$ActionQueueFileName {{.MachineTag}}{{.Namespace}}_0
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$ActionQueueMaxDiskSpace 512M
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{.DataDir}}{{.Namespace}}/ca-cert.pem
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

:syslogtag, startswith, "juju{{.Namespace}}-" @@{{.Server}}:{{.Port}};LongTagForwardFormat
# end: Forwarding rule for {{.Server}}

:syslogtag, startswith, "juju{{.Namespace}}-" stop

$FileCreateMode 0600

# Maximum size for the log on this outchannel is 512MB
# The command to execute when an outchannel as reached its size limit cannot accept any arguments
# that is why we have created the helper script for executing logrotate.
$outchannel logRotation,{{.LogDir}}{{.Namespace}}/all-machines.log,536870912,{{.DataDir}}{{.Namespace}}/logrotate.run

$RuleSet remote
$FileCreateMode 0600
:syslogtag, startswith, "juju{{.Namespace}}-" :omfile:$logRotation;JujuLogFormat{{.Namespace}}
:syslogtag, startswith, "juju{{.Namespace}}-" stop
$FileCreateMode 0600

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName {{.LogDir}}{{.Namespace}}/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor

$ModLoad imtcp
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{.DataDir}}{{.Namespace}}/ca-cert.pem
$DefaultNetstreamDriverCertFile {{.DataDir}}{{.Namespace}}/rsyslog-cert.pem
$DefaultNetstreamDriverKeyFile {{.DataDir}}{{.Namespace}}/rsyslog-key.pem
$InputTCPServerStreamDriverAuthMode anon
$InputTCPServerStreamDriverMode 1 # run driver in TLS-only mode
$InputTCPMaxSessions 10000 # default is 200, all agents connect to all rsyslog daemons

$InputTCPServerBindRuleset remote
$InputTCPServerRun {{.Port}}

# switch back to default ruleset for further rules
$RuleSet RSYSLOG_DefaultRuleset
`

type TemplateArgs struct {
	MachineTag string
	LogDir     string
	DataDir    string
	Namespace  string
	Server     string
	Port       int
	Offset     int
}

// ExpectedAccumulateSyslogConf returns the expected content for a rsyslog file on a state server.
func ExpectedAccumulateSyslogConf(c *gc.C, args TemplateArgs) string {
	if args.Namespace != "" {
		args.Namespace = "-" + args.Namespace
	}
	args.Offset = len("juju-") + len(args.Namespace) + 1
	t := template.Must(template.New("").Parse(expectedAccumulateSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, args)
	c.Assert(err, jc.ErrorIsNil)
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

# start: Forwarding rule for {{.Server}}
$ActionQueueType LinkedList
$ActionQueueFileName {{.MachineTag}}{{.Namespace}}_0
$ActionResumeRetryCount -1
$ActionQueueSaveOnShutdown on
$ActionQueueMaxDiskSpace 512M
$DefaultNetstreamDriver gtls
$DefaultNetstreamDriverCAFile {{.DataDir}}/ca-cert.pem
$ActionSendStreamDriverAuthMode anon
$ActionSendStreamDriverMode 1 # run driver in TLS-only mode

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"
:syslogtag, startswith, "juju{{.Namespace}}-" @@{{.Server}}:{{.Port}};LongTagForwardFormat
# end: Forwarding rule for {{.Server}}

& ~
`

// ExpectedForwardSyslogConf returns the expected content for a rsyslog file on a host machine.
func ExpectedForwardSyslogConf(c *gc.C, args TemplateArgs) string {
	if args.Namespace != "" {
		args.Namespace = "-" + args.Namespace
	}
	t := template.Must(template.New("").Parse(expectedForwardSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, args)
	c.Assert(err, jc.ErrorIsNil)
	return conf.String()
}
