// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

// BaseSuite provides required functionality for all test suites
// when embedded in a gocheck suite type:
// - logger redirect
// - no outgoing network access
// - protection of user's home directory
// - scrubbing of env vars
type BaseSuite struct {
	testbase.LoggingSuite
	oldHomeEnv     string
	oldEnvironment map[string]string
}

func (t *BaseSuite) SetUpSuite(c *gc.C) {
	t.LoggingSuite.SetUpSuite(c)
	t.PatchValue(&utils.OutgoingAccessAllowed, false)
}

func (t *BaseSuite) SetUpTest(c *gc.C) {
	t.oldEnvironment = make(map[string]string)
	for _, name := range []string{
		osenv.JujuHomeEnvKey,
		osenv.JujuEnvEnvKey,
		osenv.JujuLoggingConfigEnvKey,
	} {
		t.oldEnvironment[name] = os.Getenv(name)
	}
	t.oldHomeEnv = osenv.Home()
	osenv.SetHome("")
	os.Setenv(osenv.JujuHomeEnvKey, "")
	os.Setenv(osenv.JujuEnvEnvKey, "")
	os.Setenv(osenv.JujuLoggingConfigEnvKey, "")
	t.LoggingSuite.SetUpTest(c)
}

func (t *BaseSuite) TearDownTest(c *gc.C) {
	t.LoggingSuite.TearDownTest(c)
	for name, value := range t.oldEnvironment {
		os.Setenv(name, value)
	}
	osenv.SetHome(t.oldHomeEnv)
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
	osenv.SetHome(fakeHome)

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
		path := filepath.Join(osenv.Home(), f.Name)
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
	all := append([]string{osenv.Home()}, names...)
	return filepath.Join(all...)
}
