// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/deployer"
)

type SimpleContextSuite struct {
	SimpleToolsFixture
}

var _ = Suite(&SimpleContextSuite{})

func (s *SimpleContextSuite) SetUpTest(c *C) {
	s.SimpleToolsFixture.SetUp(c, c.MkDir())
}

func (s *SimpleContextSuite) TearDownTest(c *C) {
	s.SimpleToolsFixture.TearDown(c)
}

func (s *SimpleContextSuite) TestDeployRecall(c *C) {
	mgr0 := s.getContext(c)
	units, err := mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	s.assertUpstartCount(c, 0)

	err = mgr0.DeployUnit("foo/123", "some-password")
	c.Assert(err, IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{"foo/123"})
	s.assertUpstartCount(c, 1)
	s.checkUnitInstalled(c, "foo/123", "some-password")

	err = mgr0.RecallUnit("foo/123")
	c.Assert(err, IsNil)
	units, err = mgr0.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	s.assertUpstartCount(c, 0)
	s.checkUnitRemoved(c, "foo/123")
}

func (s *SimpleContextSuite) TestOldDeployedUnitsCanBeRecalled(c *C) {
	// After r1347 deployer tag is no longer part of the upstart conf filenames,
	// now only the units' tags are used. This change is with the assumption only
	// one deployer will be running on a machine (in the machine agent as a task,
	// unlike before where there was one in the unit agent as well).
	// This test ensures units deployed previously (or their upstart confs more
	// specifically) can be detected and recalled by the deployer.

	manager := s.getContext(c)

	// No deployed units at first.
	units, err := manager.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
	s.assertUpstartCount(c, 0)

	// Trying to recall any units will fail.
	err = manager.RecallUnit("principal/1")
	c.Assert(err, ErrorMatches, `unit "principal/1" is not deployed`)

	// Simulate some previously deployed units with the old
	// upstart conf filename format (+deployer tags).
	s.injectUnit(c, "jujud-machine-0:unit-mysql-0.conf", "unit-mysql-0")
	s.assertUpstartCount(c, 1)
	s.injectUnit(c, "jujud-unit-wordpress-0:unit-nrpe-0.conf", "unit-nrpe-0")
	s.assertUpstartCount(c, 2)

	// Make sure we can discover them.
	units, err = manager.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 2)
	sort.Strings(units)
	c.Assert(units, DeepEquals, []string{"mysql/0", "nrpe/0"})

	// Deploy some units.
	err = manager.DeployUnit("principal/1", "some-password")
	c.Assert(err, IsNil)
	s.checkUnitInstalled(c, "principal/1", "some-password")
	s.assertUpstartCount(c, 3)
	err = manager.DeployUnit("subordinate/2", "fake-password")
	c.Assert(err, IsNil)
	s.checkUnitInstalled(c, "subordinate/2", "fake-password")
	s.assertUpstartCount(c, 4)

	// Verify the newly deployed units are also discoverable.
	units, err = manager.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 4)
	sort.Strings(units)
	c.Assert(units, DeepEquals, []string{"mysql/0", "nrpe/0", "principal/1", "subordinate/2"})

	// Recall all of them - should work ok.
	unitCount := 4
	for _, unitName := range units {
		err = manager.RecallUnit(unitName)
		c.Assert(err, IsNil)
		unitCount--
		s.checkUnitRemoved(c, unitName)
		s.assertUpstartCount(c, unitCount)
	}

	// Verify they're no longer discoverable.
	units, err = manager.DeployedUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
}

type SimpleToolsFixture struct {
	dataDir         string
	initDir         string
	logDir          string
	origPath        string
	binDir          string
	syslogConfigDir string
}

var fakeJujud = "#!/bin/bash\n# fake-jujud\nexit 0\n"

func (fix *SimpleToolsFixture) SetUp(c *C, dataDir string) {
	fix.dataDir = dataDir
	fix.initDir = c.MkDir()
	fix.logDir = c.MkDir()
	fix.syslogConfigDir = c.MkDir()
	toolsDir := agent.SharedToolsDir(fix.dataDir, version.Current)
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, IsNil)
	jujudPath := filepath.Join(toolsDir, "jujud")
	err = ioutil.WriteFile(jujudPath, []byte(fakeJujud), 0755)
	c.Assert(err, IsNil)
	urlPath := filepath.Join(toolsDir, "downloaded-url.txt")
	err = ioutil.WriteFile(urlPath, []byte("http://testing.invalid/tools"), 0644)
	c.Assert(err, IsNil)
	fix.binDir = c.MkDir()
	fix.origPath = os.Getenv("PATH")
	os.Setenv("PATH", fix.binDir+":"+fix.origPath)
	fix.makeBin(c, "status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "stopped-status", `echo "blah stop/waiting"`)
	fix.makeBin(c, "started-status", `echo "blah start/running, process 666"`)
	fix.makeBin(c, "start", "cp $(which started-status) $(which status)")
	fix.makeBin(c, "stop", "cp $(which stopped-status) $(which status)")
}

func (fix *SimpleToolsFixture) TearDown(c *C) {
	os.Setenv("PATH", fix.origPath)
}

func (fix *SimpleToolsFixture) makeBin(c *C, name, script string) {
	path := filepath.Join(fix.binDir, name)
	err := ioutil.WriteFile(path, []byte("#!/bin/bash\n"+script), 0755)
	c.Assert(err, IsNil)
}

func (fix *SimpleToolsFixture) assertUpstartCount(c *C, count int) {
	fis, err := ioutil.ReadDir(fix.initDir)
	c.Assert(err, IsNil)
	c.Assert(fis, HasLen, count)
}

func (fix *SimpleToolsFixture) getContext(c *C) *deployer.SimpleContext {
	return deployer.NewTestSimpleContext(fix.initDir, fix.dataDir, fix.logDir, fix.syslogConfigDir)
}

func (fix *SimpleToolsFixture) paths(tag string) (confPath, agentDir, toolsDir, syslogConfPath string) {
	confName := fmt.Sprintf("jujud-%s.conf", tag)
	confPath = filepath.Join(fix.initDir, confName)
	agentDir = agent.Dir(fix.dataDir, tag)
	toolsDir = agent.ToolsDir(fix.dataDir, tag)
	syslogConfPath = filepath.Join(fix.syslogConfigDir, fmt.Sprintf("26-juju-%s.conf", tag))
	return
}

var expectedSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-%s-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/%s.log
$InputFileTag juju-%s:
$InputFileStateFile %s
$InputRunFileMonitor

:syslogtag, startswith, "juju-" @s1:514
& ~
`

func (fix *SimpleToolsFixture) checkUnitInstalled(c *C, name, password string) {
	tag := state.UnitTag(name)
	uconfPath, _, toolsDir, syslogConfPath := fix.paths(tag)
	uconfData, err := ioutil.ReadFile(uconfPath)
	c.Assert(err, IsNil)
	uconf := string(uconfData)
	var execLine string
	for _, line := range strings.Split(uconf, "\n") {
		if strings.HasPrefix(line, "exec ") {
			execLine = line
			break
		}
	}
	if execLine == "" {
		c.Fatalf("no command found in %s:\n%s", uconfPath, uconf)
	}
	logPath := filepath.Join(fix.logDir, tag+".log")
	jujudPath := filepath.Join(toolsDir, "jujud")
	for _, pat := range []string{
		"^exec " + jujudPath + " unit ",
		" --unit-name " + name + " ",
		" >> " + logPath + " 2>&1$",
	} {
		match, err := regexp.MatchString(pat, execLine)
		c.Assert(err, IsNil)
		if !match {
			c.Fatalf("failed to match:\n%s\nin:\n%s", pat, execLine)
		}
	}

	conf, err := agent.ReadConf(fix.dataDir, tag)
	c.Assert(err, IsNil)
	c.Assert(conf, DeepEquals, &agent.Conf{
		DataDir:     fix.dataDir,
		OldPassword: password,
		StateInfo: &state.Info{
			Addrs:  []string{"s1:123", "s2:123"},
			CACert: []byte("test-cert"),
			Tag:    tag,
		},
		APIInfo: &api.Info{
			Addrs:  []string{"a1:123", "a2:123"},
			CACert: []byte("test-cert"),
			Tag:    tag,
		},
	})

	jujudData, err := ioutil.ReadFile(jujudPath)
	c.Assert(err, IsNil)
	c.Assert(string(jujudData), Equals, fakeJujud)

	syslogConfData, err := ioutil.ReadFile(syslogConfPath)
	c.Assert(err, IsNil)
	parts := strings.SplitN(name, "/", 2)
	unitTag := fmt.Sprintf("unit-%s-%s", parts[0], parts[1])
	expectedSyslogConfReplaced := fmt.Sprintf(expectedSyslogConf, unitTag, unitTag, unitTag, unitTag)
	c.Assert(string(syslogConfData), Equals, expectedSyslogConfReplaced)

}

func (fix *SimpleToolsFixture) checkUnitRemoved(c *C, name string) {
	tag := state.UnitTag(name)
	confPath, agentDir, toolsDir, syslogConfPath := fix.paths(tag)
	for _, path := range []string{confPath, agentDir, toolsDir, syslogConfPath} {
		_, err := ioutil.ReadFile(path)
		if err == nil {
			c.Log("Warning: %q not removed as expected", path)
		} else {
			c.Assert(err, checkers.Satisfies, os.IsNotExist)
		}
	}
}

func (fix *SimpleToolsFixture) injectUnit(c *C, upstartConf, unitTag string) {
	confPath := filepath.Join(fix.initDir, upstartConf)
	err := ioutil.WriteFile(confPath, []byte("#!/bin/bash\necho $0"), 0644)
	c.Assert(err, IsNil)
	toolsDir := filepath.Join(fix.dataDir, "tools", unitTag)
	err = os.MkdirAll(toolsDir, 0755)
	c.Assert(err, IsNil)
}
