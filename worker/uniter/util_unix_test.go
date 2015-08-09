// Copyright 2012-2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package uniter_test

import (
	"fmt"
	"path/filepath"

	gc "gopkg.in/check.v1"
)

// Command suffix for the hooks
var cmdSuffix = ""

var (
	// Variables for changed hooks. These are used in uniter_test
	appendConfigChanged            = "config-get --format yaml --output config.out"
	uniterRelationsCustomizeScript = "relation-ids db > relations.out && chmod 644 relations.out"
)

var (
	// Used in TestLeadership
	leadershipScript = `
if [ $(is-leader) != "False" ]; then exit -1; fi
`[1:]

	// Different hook file contents. These are used in util_test
	goodHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID %s $JUJU_REMOTE_UNIT
`[1:]

	badHook = `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID fail-%s $JUJU_REMOTE_UNIT
exit 1
`[1:]

	rebootHook = `
#!/bin/bash --norc
juju-reboot
`[1:]

	badRebootHook = `
#!/bin/bash --norc
juju-reboot
exit 1
`[1:]

	rebootNowHook = `
#!/bin/bash --norc

if [ -f "i_have_risen" ]
then
    exit 0
fi
touch i_have_risen
juju-reboot --now
`[1:]

	// Map of action files contents. These are used in util_test
	actions = map[string]string{
		"action-log": `
#!/bin/bash --norc
juju-log $JUJU_ENV_UUID action-log
`[1:],
		"snapshot": `
#!/bin/bash --norc
action-set outfile.name="snapshot-01.tar" outfile.size="10.3GB"
action-set outfile.size.magnitude="10.3" outfile.size.units="GB"
action-set completion.status="yes" completion.time="5m"
action-set completion="yes"
`[1:],
		"action-log-fail": `
#!/bin/bash --norc
action-fail "I'm afraid I can't let you do that, Dave."
action-set foo="still works"
`[1:],
		"action-log-fail-error": `
#!/bin/bash --norc
action-fail too many arguments
action-set foo="still works"
action-fail "A real message"
`[1:],
		"action-reboot": `
#!/bin/bash --norc
juju-reboot || action-set reboot-delayed="good"
juju-reboot --now || action-set reboot-now="good"
`[1:],
	}
)

func echoUnitNameToFileHelper(testDir, name string) string {
	path := filepath.Join(testDir, name)
	template := "echo juju run ${JUJU_UNIT_NAME} > %s.tmp; mv %s.tmp %s"
	return fmt.Sprintf(template, path, path, path)
}

func (s *UniterSuite) TestRunCommand(c *gc.C) {
	testDir := c.MkDir()
	testFile := func(name string) string {
		return filepath.Join(testDir, name)
	}
	echoUnitNameToFile := func(name string) string {
		return echoUnitNameToFileHelper(testDir, name)
	}

	s.runUniterTests(c, []uniterTest{
		ut(
			"run commands: environment",
			quickStart{},
			runCommands{echoUnitNameToFile("run.output")},
			verifyFile{filepath.Join(testDir, "run.output"), "juju run u/0\n"},
		), ut(
			"run commands: jujuc commands",
			quickStartRelation{},
			runCommands{
				fmt.Sprintf("unit-get private-address >> %s", testFile("jujuc.output")),
				fmt.Sprintf("unit-get public-address >> %s", testFile("jujuc.output")),
			},
			verifyFile{
				testFile("jujuc.output"),
				"private.address.example.com\npublic.address.example.com\n",
			},
		), ut(
			"run commands: jujuc environment",
			quickStartRelation{},
			relationRunCommands{
				fmt.Sprintf("echo $JUJU_RELATION_ID > %s", testFile("jujuc-env.output")),
				fmt.Sprintf("echo $JUJU_REMOTE_UNIT >> %s", testFile("jujuc-env.output")),
			},
			verifyFile{
				testFile("jujuc-env.output"),
				"db:0\nmysql/0\n",
			},
		), ut(
			"run commands: proxy settings set",
			quickStartRelation{},
			setProxySettings{Http: "http", Https: "https", Ftp: "ftp", NoProxy: "localhost"},
			runCommands{
				fmt.Sprintf("echo $http_proxy > %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTP_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $https_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $HTTPS_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $ftp_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $FTP_PROXY >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $no_proxy >> %s", testFile("proxy.output")),
				fmt.Sprintf("echo $NO_PROXY >> %s", testFile("proxy.output")),
			},
			verifyFile{
				testFile("proxy.output"),
				"http\nhttp\nhttps\nhttps\nftp\nftp\nlocalhost\nlocalhost\n",
			},
		), ut(
			"run commands: async using rpc client",
			quickStart{},
			asyncRunCommands{echoUnitNameToFile("run.output")},
			verifyFile{testFile("run.output"), "juju run u/0\n"},
			waitContextWaitGroup{},
		), ut(
			"run commands: waits for lock",
			quickStart{},
			acquireHookSyncLock{},
			asyncRunCommands{echoUnitNameToFile("wait.output")},
			verifyNoFile{testFile("wait.output")},
			releaseHookSyncLock,
			verifyFile{testFile("wait.output"), "juju run u/0\n"},
			waitContextWaitGroup{},
		),
	})
}
