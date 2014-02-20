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
$InputFileName /var/log/juju/{{machine}}.log
$InputFileTag juju{{namespace}}-{{machine}}:
$InputFileStateFile {{machine}}{{namespace}}
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun {{port}}

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat{{namespace}},"%syslogtag:{{offset}}:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

$FileCreateMode 0644
:syslogtag, startswith, "juju{{namespace}}-" /var/log/juju{{namespace}}/all-machines.log;JujuLogFormat{{namespace}}
& ~
$FileCreateMode 0640
`

// ExpectedAccumulateSyslogConf returns the expected content for a rsyslog file on a state server.
func ExpectedAccumulateSyslogConf(c *gc.C, machineTag, namespace string, port int) string {
	if namespace != "" {
		namespace = "-" + namespace
	}
	t := template.New("")
	t.Funcs(template.FuncMap{
		"machine":   func() string { return machineTag },
		"namespace": func() string { return namespace },
		"port":      func() int { return port },
		"offset":    func() int { return 6 + len(namespace) },
	})
	t = template.Must(t.Parse(expectedAccumulateSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, nil)
	c.Assert(err, gc.IsNil)
	return conf.String()
}

var expectedForwardSyslogConfTemplate = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/{{machine}}.log
$InputFileTag juju{{namespace}}-{{machine}}:
$InputFileStateFile {{machine}}{{namespace}}
$InputRunFileMonitor

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

:syslogtag, startswith, "juju{{namespace}}-" @{{bootstrapIP}}:{{port}};LongTagForwardFormat
& ~
`

// ExpectedForwardSyslogConf returns the expected content for a rsyslog file on a host machine.
func ExpectedForwardSyslogConf(c *gc.C, machineTag, namespace, bootstrapIP string, port int) string {
	if namespace != "" {
		namespace = "-" + namespace
	}
	t := template.New("")
	t.Funcs(template.FuncMap{
		"machine":     func() string { return machineTag },
		"namespace":   func() string { return namespace },
		"bootstrapIP": func() string { return bootstrapIP },
		"port":        func() int { return port },
	})
	t = template.Must(t.Parse(expectedForwardSyslogConfTemplate))
	var conf bytes.Buffer
	err := t.Execute(&conf, nil)
	c.Assert(err, gc.IsNil)
	return conf.String()
}
