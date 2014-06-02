// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/utils"
)

// BaseSuite provides required functionality for all test suites
// when embedded in a gocheck suite type:
// - logger redirect
// - no outgoing network access
// - protection of user's home directory
// - scrubbing of env vars
type BaseSuite struct {
	testing.CleanupSuite
	testing.LoggingSuite
	oldHomeEnv     string
	oldEnvironment map[string]string
}

func (t *BaseSuite) SetUpSuite(c *gc.C) {
	t.CleanupSuite.SetUpSuite(c)
	t.LoggingSuite.SetUpSuite(c)
	t.PatchValue(&utils.OutgoingAccessAllowed, false)
}

func (t *BaseSuite) TearDownSuite(c *gc.C) {
	t.LoggingSuite.TearDownSuite(c)
	t.CleanupSuite.TearDownSuite(c)
}

func (t *BaseSuite) SetUpTest(c *gc.C) {
	t.CleanupSuite.SetUpTest(c)
	t.LoggingSuite.SetUpTest(c)
	t.oldEnvironment = make(map[string]string)
	for _, name := range []string{
		osenv.JujuHomeEnvKey,
		osenv.JujuEnvEnvKey,
		osenv.JujuLoggingConfigEnvKey,
	} {
		t.oldEnvironment[name] = os.Getenv(name)
	}
	t.oldHomeEnv = utils.Home()
	utils.SetHome("")
	os.Setenv(osenv.JujuHomeEnvKey, "")
	os.Setenv(osenv.JujuEnvEnvKey, "")
	os.Setenv(osenv.JujuLoggingConfigEnvKey, "")
}

func (t *BaseSuite) TearDownTest(c *gc.C) {
	for name, value := range t.oldEnvironment {
		os.Setenv(name, value)
	}
	utils.SetHome(t.oldHomeEnv)
	t.LoggingSuite.TearDownTest(c)
	t.CleanupSuite.TearDownTest(c)
}

type TestFile struct {
	Name, Data string
}

// FakeHome stores information about the user's home
// environment so it can be cast aside for tests and
// restored afterwards.
type FakeHome struct {
	files []TestFile
}

func MakeFakeHome(c *gc.C) *FakeHome {
	fakeHome := c.MkDir()
	utils.SetHome(fakeHome)

	sshPath := filepath.Join(fakeHome, ".ssh")
	err := os.Mkdir(sshPath, 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(filepath.Join(sshPath, "id_rsa.pub"), []byte("auth key\n"), 0666)
	c.Assert(err, gc.IsNil)

	return &FakeHome{
		files: []TestFile{},
	}
}

func (h *FakeHome) AddFiles(c *gc.C, files ...TestFile) {
	for _, f := range files {
		path := filepath.Join(utils.Home(), f.Name)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(path, []byte(f.Data), 0666)
		c.Assert(err, gc.IsNil)
		h.files = append(h.files, f)
	}
}

// FileContents returns the test file contents for the
// given specified path (which may be relative, so
// we compare with the base filename only).
func (h *FakeHome) FileContents(c *gc.C, path string) string {
	for _, f := range h.files {
		if filepath.Base(f.Name) == filepath.Base(path) {
			return f.Data
		}
	}
	c.Fatalf("path attribute holds unknown test file: %q", path)
	panic("unreachable")
}

// FileExists returns if the given relative file path exists
// in the fake home.
func (h *FakeHome) FileExists(path string) bool {
	for _, f := range h.files {
		if f.Name == path {
			return true
		}
	}
	return false
}

// HomePath joins the specified path snippets and returns
// an absolute path under Juju home.
func HomePath(names ...string) string {
	all := append([]string{utils.Home()}, names...)
	return filepath.Join(all...)
}
