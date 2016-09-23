// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type steps125Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps125Suite{})

func (s *steps125Suite) TestStateStepsFor125(c *gc.C) {
	expected := []string{
		"set hosted environment count to number of hosted environments",
		"tag machine instances",
		"add missing env-uuid to statuses",
		"add attachmentCount to volume",
		"add attachmentCount to filesystem",
		"add binding to volume",
		"add binding to filesystem",
		"add status to volume",
		"add preferred addresses to machines",
		"upgrade environment config",
		"move lastlogin and last connection to their own collections",
		"sets an unknown unit status for missing unit statuses",
	}
	assertStateSteps(c, version.MustParse("1.25.0"), expected)
}

func (s *steps125Suite) TestStateStepsFor1252(c *gc.C) {
	expected := []string{
		"remove invalid tools metadata from state",
	}
	assertStateSteps(c, version.MustParse("1.25.2"), expected)
}

func (s *steps125Suite) TestStepsFor125(c *gc.C) {
	expected := []string{
		"remove Jujud.pass file on windows",
		"add juju registry key",
	}
	assertSteps(c, version.MustParse("1.25.0"), expected)
}

func (s *steps125Suite) TestStepsFor1257(c *gc.C) {
	expected := []string{
		"remove apiserver charm get cache",
	}
	assertSteps(c, version.MustParse("1.25.7"), expected)
}

type mockOSRemove struct {
	called     bool
	path       string
	shouldFail bool
}

func (m *mockOSRemove) osRemove(path string) error {
	m.called = true
	m.path = path
	if m.shouldFail {
		return errors.New("i done error'd")
	}
	return nil
}

var removeFileTests = []struct {
	os           version.OSType
	callExpected bool
	shouldFail   bool
}{
	{
		os:           version.Ubuntu,
		callExpected: false,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   true,
	},
}

func (s *steps125Suite) TestRemoveJujudPass(c *gc.C) {
	for _, t := range removeFileTests {
		mock := &mockOSRemove{shouldFail: t.shouldFail}
		s.PatchValue(upgrades.OsRemove, mock.osRemove)
		s.PatchValue(&version.Current.OS, t.os)
		err := upgrades.RemoveJujudpass(nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mock.called, gc.Equals, t.callExpected)
	}
}

type mockRunCmds struct {
	c          *gc.C
	commands   string
	called     bool
	shouldFail bool
}

func (m *mockRunCmds) runCommands(params exec.RunParams) (*exec.ExecResponse, error) {
	m.called = true
	m.c.Assert(params.Commands, gc.Equals, strings.Join(cloudconfig.CreateJujuRegistryKeyCmds(), "\n"))
	if m.shouldFail {
		return nil, errors.New("derp")
	}
	return nil, nil
}

var addRegKeyTests = []struct {
	os           version.OSType
	callExpected bool
	shouldFail   bool
	errMessage   string
}{
	{
		os:           version.Ubuntu,
		callExpected: false,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   true,
		errMessage:   "could not create juju registry key: derp",
	},
}

func (s *steps125Suite) TestAddJujuRegKey(c *gc.C) {
	for _, t := range addRegKeyTests {
		mock := &mockRunCmds{shouldFail: t.shouldFail, c: c}
		s.PatchValue(upgrades.ExecRunCommands, mock.runCommands)
		s.PatchValue(&version.Current.OS, t.os)
		err := upgrades.AddJujuRegKey(nil)
		if t.shouldFail {
			c.Assert(err, gc.ErrorMatches, t.errMessage)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Assert(mock.called, gc.Equals, t.callExpected)
	}
}
func (s *steps125Suite) TestCharmGetCacheDir(c *gc.C) {
	// Create a cache directory with some stuff in it.
	dataDir := c.MkDir()
	cacheDir := filepath.Join(dataDir, "charm-get-cache")
	c.Assert(os.MkdirAll(cacheDir, 0777), jc.ErrorIsNil)
	err := ioutil.WriteFile(filepath.Join(cacheDir, "stuff"), []byte("things"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	check := func() {
		context := &mockContext{
			agentConfig: &mockAgentConfig{dataDir: dataDir},
		}
		err := upgrades.RemoveCharmGetCache(context)
		c.Assert(err, jc.ErrorIsNil)

		// Cache directory should be gone, but data dir should still be there.
		c.Check(pathExists(cacheDir), jc.IsFalse)
		c.Check(pathExists(dataDir), jc.IsTrue)
	}

	check()
	check() // Check OK when directory not present
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	if err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	}
	panic(fmt.Sprintf("stat for %q failed: %v", p, err))
}
