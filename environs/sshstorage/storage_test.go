// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
)

type storageSuite struct {
	testing.LoggingSuite
	restoreEnv func()
}

var _ = gc.Suite(&storageSuite{})

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

var sshCommandOrig = sshCommand

func sshCommandTesting(host string, tty bool, command string) *exec.Cmd {
	cmd := exec.Command("bash", "-c", command)
	uid := fmt.Sprint(os.Getuid())
	gid := fmt.Sprint(os.Getgid())
	defer testing.PatchEnvironment("SUDO_UID", uid)()
	defer testing.PatchEnvironment("SUDO_GID", gid)()
	cmd.Env = os.Environ()
	return cmd
}

func (s *storageSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	// Create a "sudo" command which just executes its args.
	bin := c.MkDir()
	c.Assert(os.Symlink("/usr/bin/env", filepath.Join(bin, "sudo")), gc.IsNil)
	s.restoreEnv = testing.PatchEnvironment("PATH", bin+":"+os.Getenv("PATH"))
	sshCommand = sshCommandTesting
}

func (s *storageSuite) TearDownSuite(c *gc.C) {
	sshCommand = sshCommandOrig
	s.restoreEnv()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *storageSuite) TestNewSSHStorage(c *gc.C) {
	storageDir := c.MkDir()
	for i := 0; i < 2; i++ {
		storage, err := NewSSHStorage("example.com", storageDir)
		c.Assert(err, gc.IsNil)
		c.Assert(storage, gc.NotNil)
		c.Assert(storage.Close(), gc.IsNil)
	}

	// You must have permissions to create the directory.
	c.Assert(os.Chmod(storageDir, 0555), gc.IsNil)
	_, err := NewSSHStorage("example.com", filepath.Join(storageDir, "subdir"))
	c.Assert(err, gc.ErrorMatches, ".*cannot create directory.*")
}

func (s *storageSuite) TestPathValidity(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()

	c.Assert(os.Mkdir(filepath.Join(storageDir, "a"), 0755), gc.IsNil)
	f, err := os.Create(filepath.Join(storageDir, "a", "b"))
	c.Assert(err, gc.IsNil)
	c.Assert(f.Close(), gc.IsNil)

	for _, prefix := range []string{"..", "a/../.."} {
		c.Logf("prefix: %q", prefix)
		_, err := storage.List(prefix)
		c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(fmt.Sprintf("%q escapes storage directory", prefix)))
	}

	// Paths are always relative, so a leading "/" may as well not be there.
	names, err := storage.List("/")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"a/b"})

	// Paths will be canonicalised.
	names, err = storage.List("a/..")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"a/b"})
}

func (s *storageSuite) TestGet(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	data := []byte("abc\000def")
	c.Assert(os.Mkdir(filepath.Join(storageDir, "a"), 0755), gc.IsNil)
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err = ioutil.WriteFile(filepath.Join(storageDir, name), data, 0644)
		c.Assert(err, gc.IsNil)
		r, err := storage.Get(name)
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.DeepEquals, data)
	}

	_, err = storage.Get("notthere")
	c.Assert(err, jc.Satisfies, coreerrors.IsNotFoundError)
}

func (s *storageSuite) TestPut(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	data := []byte("abc\000def")
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err = storage.Put(name, bytes.NewBuffer(data), int64(len(data)))
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadFile(filepath.Join(storageDir, name))
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.DeepEquals, data)
	}
}

func (s *storageSuite) assertList(c *gc.C, storage environs.StorageReader, prefix string, expected []string) {
	c.Logf("List: %v", prefix)
	names, err := storage.List(prefix)
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, expected)
}

func (s *storageSuite) TestList(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	s.assertList(c, storage, "", nil)

	// Directories don't show up in List.
	c.Assert(os.Mkdir(filepath.Join(storageDir, "a"), 0755), gc.IsNil)
	s.assertList(c, storage, "", nil)
	s.assertList(c, storage, "a", nil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b2"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "b"), nil, 0), gc.IsNil)
	s.assertList(c, storage, "", []string{"a/b1", "a/b2", "b"})
	s.assertList(c, storage, "a", []string{"a/b1", "a/b2"})
	s.assertList(c, storage, "a/b", []string{"a/b1", "a/b2"})
	s.assertList(c, storage, "a/b1", []string{"a/b1"})
	s.assertList(c, storage, "a/b3", nil)
	s.assertList(c, storage, "a/b/c", nil)
	s.assertList(c, storage, "b", []string{"b"})
}

func (s *storageSuite) TestRemove(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()

	c.Assert(os.Mkdir(filepath.Join(storageDir, "a"), 0755), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b2"), nil, 0), gc.IsNil)
	c.Assert(storage.Remove("a"), gc.ErrorMatches, "rm: cannot remove.*Is a directory")
	s.assertList(c, storage, "", []string{"a/b1", "a/b2"})
	c.Assert(storage.Remove("a/b"), gc.IsNil) // doesn't exist; not an error
	s.assertList(c, storage, "", []string{"a/b1", "a/b2"})
	c.Assert(storage.Remove("a/b2"), gc.IsNil)
	s.assertList(c, storage, "", []string{"a/b1"})
	c.Assert(storage.Remove("a/b1"), gc.IsNil)
	s.assertList(c, storage, "", nil)
}

func (s *storageSuite) TestRemoveAll(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()

	c.Assert(os.Mkdir(filepath.Join(storageDir, "a"), 0755), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, "a", "b2"), nil, 0), gc.IsNil)
	s.assertList(c, storage, "", []string{"a/b1", "a/b2"})
	c.Assert(storage.RemoveAll(), gc.IsNil)
	s.assertList(c, storage, "", nil)

	// RemoveAll does not remove the base storage directory.
	_, err = os.Stat(storageDir)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestURL(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	url, err := storage.URL("a/b")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "sftp://example.com/"+path.Join(storageDir, "a/b"))
}

func (s *storageSuite) TestConsistencyStrategy(c *gc.C) {
	storageDir := c.MkDir()
	storage, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	c.Assert(storage.ConsistencyStrategy(), gc.Equals, utils.AttemptStrategy{})
}
