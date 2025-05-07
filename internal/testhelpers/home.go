// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"
)

type TestFile struct {
	Name, Data string
}

// FakeHome stores information about the user's home
// environment so it can be cast aside for tests and
// restored afterwards.
type FakeHome struct {
	files []TestFile
}

func MakeFakeHome(c *tc.C) *FakeHome {
	fakeHome := c.MkDir()
	err := utils.SetHome(fakeHome)
	c.Assert(err, tc.ErrorIsNil)

	sshPath := filepath.Join(fakeHome, ".ssh")
	err = os.Mkdir(sshPath, 0777)
	c.Assert(err, tc.IsNil)
	err = ioutil.WriteFile(filepath.Join(sshPath, "id_rsa"), []byte("private auth key\n"), 0600)
	c.Assert(err, tc.IsNil)
	err = ioutil.WriteFile(filepath.Join(sshPath, "id_rsa.pub"), []byte("public auth key\n"), 0666)
	c.Assert(err, tc.IsNil)

	return &FakeHome{
		files: []TestFile{},
	}
}

func (h *FakeHome) AddFiles(c *tc.C, files ...TestFile) {
	for _, f := range files {
		path := filepath.Join(utils.Home(), f.Name)
		err := os.MkdirAll(filepath.Dir(path), 0700)
		c.Assert(err, tc.IsNil)
		err = ioutil.WriteFile(path, []byte(f.Data), 0666)
		c.Assert(err, tc.IsNil)
		h.files = append(h.files, f)
	}
}

// FileContents returns the test file contents for the
// given specified path (which may be relative, so
// we compare with the base filename only).
func (h *FakeHome) FileContents(c *tc.C, path string) string {
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

// JujuXDGDataHomePath returns the test home path, it is just a convenience
// for tests, if extra path snippets are passed they will be
// joined to juju home.
// This tool assumes ~/.config/juju as the juju home.
func JujuXDGDataHomePath(names ...string) string {
	all := append([]string{".local", "share", "juju"}, names...)
	return HomePath(all...)
}

// FakeHomeSuite sets up a fake home directory before running tests.
type FakeHomeSuite struct {
	CleanupSuite
	LoggingSuite
	Home *FakeHome
}

func (s *FakeHomeSuite) SetUpSuite(c *tc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
}

func (s *FakeHomeSuite) TearDownSuite(c *tc.C) {
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *FakeHomeSuite) SetUpTest(c *tc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	home := utils.Home()
	s.Home = MakeFakeHome(c)
	s.AddCleanup(func(*tc.C) {
		err := utils.SetHome(home)
		c.Assert(err, tc.ErrorIsNil)
	})
}

func (s *FakeHomeSuite) TearDownTest(c *tc.C) {
	s.LoggingSuite.TearDownTest(c)
	s.CleanupSuite.TearDownTest(c)
}
