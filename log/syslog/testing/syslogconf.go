// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"text/template"

	gc "launchpad.net/gocheck"
)

var expectedAccumulateSyslogConfTemplate = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun {{.Port}}

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{.Namespace}},"%syslogtag:{{.Offset}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$FileCreateMode 0644
:syslogtag, startswith, "juju{{.Namespace}}-" /var/log/juju{{.Namespace}}/all-machines.log;JujuLogFormat{{.Namespace}}
& ~
$FileCreateMode 0640
`

type templateArgs struct {
	MachineTag  string
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
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/{{.MachineTag}}.log
$InputFileTag juju{{.Namespace}}-{{.MachineTag}}:
$InputFileStateFile {{.MachineTag}}{{.Namespace}}
$InputRunFileMonitor

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

:syslogtag, startswith, "juju{{.Namespace}}-" @{{.BootstrapIP}}:{{.Port}};LongTagForwardFormat
& ~
`

// ExpectedForwardSyslogConf returns the expected content for a rsyslog file on a host machine.
func ExpectedForwardSyslogConf(c *gc.C, machineTag, namespace, bootstrapIP string, port int) string {
	if namespace != "" {
		namespace = "-" + namespace
	}
	t := template.Must(template.New("").Parse(expectedForwardSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, templateArgs{
		MachineTag:  machineTag,
		Namespace:   namespace,
		BootstrapIP: bootstrapIP,
		Port:        port,
	})
	c.Assert(err, gc.IsNil)
	return conf.String()
}
