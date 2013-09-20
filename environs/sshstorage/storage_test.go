// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/storage"
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

// flockBin is the path to the original "flock" binary.
var flockBin string

func (s *storageSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)

	var err error
	flockBin, err = exec.LookPath("flock")
	c.Assert(err, gc.IsNil)

	bin := c.MkDir()
	s.restoreEnv = testing.PatchEnvironment("PATH", bin+":"+os.Getenv("PATH"))

	// Create a "sudo" command which just executes its args.
	c.Assert(os.Symlink("/usr/bin/env", filepath.Join(bin, "sudo")), gc.IsNil)
	sshCommand = sshCommandTesting

	// Create a new "flock" which calls the original, but in non-blocking mode.
	data := []byte(fmt.Sprintf("#!/bin/sh\nexec %s --nonblock \"$@\"", flockBin))
	c.Assert(ioutil.WriteFile(filepath.Join(bin, "flock"), data, 0755), gc.IsNil)
}

func (s *storageSuite) TearDownSuite(c *gc.C) {
	sshCommand = sshCommandOrig
	s.restoreEnv()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *storageSuite) TestNewSSHStorage(c *gc.C) {
	storageDir := c.MkDir()
	for i := 0; i < 2; i++ {
		stor, err := NewSSHStorage("example.com", storageDir)
		c.Assert(err, gc.IsNil)
		c.Assert(stor, gc.NotNil)
		c.Assert(stor.Close(), gc.IsNil)
	}
	c.Assert(os.RemoveAll(storageDir), gc.IsNil)

	// You must have permissions to create the directory.
	storageDir = c.MkDir()
	c.Assert(os.Chmod(storageDir, 0555), gc.IsNil)
	_, err := NewSSHStorage("example.com", filepath.Join(storageDir))
	c.Assert(err, gc.ErrorMatches, "(.|\n)*cannot change owner and permissions of(.|\n)*")
}

func (s *storageSuite) TestPathValidity(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()

	c.Assert(os.Mkdir(filepath.Join(storageDir, contentdir, "a"), 0755), gc.IsNil)
	f, err := os.Create(filepath.Join(storageDir, contentdir, "a", "b"))
	c.Assert(err, gc.IsNil)
	c.Assert(f.Close(), gc.IsNil)

	for _, prefix := range []string{"..", "a/../.."} {
		c.Logf("prefix: %q", prefix)
		_, err := storage.List(stor, prefix)
		c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(fmt.Sprintf("%q escapes storage directory", prefix)))
	}

	// Paths are always relative, so a leading "/" may as well not be there.
	names, err := storage.List(stor, "/")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"a/b"})

	// Paths will be canonicalised.
	names, err = storage.List(stor, "a/..")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"a/b"})
}

func (s *storageSuite) TestGet(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	data := []byte("abc\000def")
	c.Assert(os.Mkdir(filepath.Join(storageDir, contentdir, "a"), 0755), gc.IsNil)
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err = ioutil.WriteFile(filepath.Join(storageDir, contentdir, name), data, 0644)
		c.Assert(err, gc.IsNil)
		r, err := storage.Get(stor, name)
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.DeepEquals, data)
	}

	_, err = storage.Get(stor, "notthere")
	c.Assert(err, jc.Satisfies, coreerrors.IsNotFoundError)
}

func (s *storageSuite) TestPut(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	data := []byte("abc\000def")
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err = stor.Put(name, bytes.NewBuffer(data), int64(len(data)))
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadFile(filepath.Join(storageDir, contentdir, name))
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.DeepEquals, data)
	}
}

func (s *storageSuite) assertList(c *gc.C, stor storage.StorageReader, prefix string, expected []string) {
	c.Logf("List: %v", prefix)
	names, err := storage.List(stor, prefix)
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, expected)
}

func (s *storageSuite) TestList(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	s.assertList(c, stor, "", nil)

	// Directories don't show up in List.
	contentDir := filepath.Join(storageDir, contentdir)
	c.Assert(os.Mkdir(filepath.Join(contentDir, "a"), 0755), gc.IsNil)
	s.assertList(c, stor, "", nil)
	s.assertList(c, stor, "a", nil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b2"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "b"), nil, 0), gc.IsNil)
	s.assertList(c, stor, "", []string{"a/b1", "a/b2", "b"})
	s.assertList(c, stor, "a", []string{"a/b1", "a/b2"})
	s.assertList(c, stor, "a/b", []string{"a/b1", "a/b2"})
	s.assertList(c, stor, "a/b1", []string{"a/b1"})
	s.assertList(c, stor, "a/b3", nil)
	s.assertList(c, stor, "a/b/c", nil)
	s.assertList(c, stor, "b", []string{"b"})
}

func (s *storageSuite) TestRemove(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()

	contentDir := filepath.Join(storageDir, contentdir)
	c.Assert(os.Mkdir(filepath.Join(contentDir, "a"), 0755), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b2"), nil, 0), gc.IsNil)
	c.Assert(stor.Remove("a"), gc.ErrorMatches, "rm: cannot remove.*Is a directory")
	s.assertList(c, stor, "", []string{"a/b1", "a/b2"})
	c.Assert(stor.Remove("a/b"), gc.IsNil) // doesn't exist; not an error
	s.assertList(c, stor, "", []string{"a/b1", "a/b2"})
	c.Assert(stor.Remove("a/b2"), gc.IsNil)
	s.assertList(c, stor, "", []string{"a/b1"})
	c.Assert(stor.Remove("a/b1"), gc.IsNil)
	s.assertList(c, stor, "", nil)
}

func (s *storageSuite) TestRemoveAll(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()

	contentDir := filepath.Join(storageDir, contentdir)
	c.Assert(os.Mkdir(filepath.Join(contentDir, "a"), 0755), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b1"), nil, 0), gc.IsNil)
	c.Assert(ioutil.WriteFile(filepath.Join(contentDir, "a", "b2"), nil, 0), gc.IsNil)
	s.assertList(c, stor, "", []string{"a/b1", "a/b2"})
	c.Assert(stor.RemoveAll(), gc.IsNil)
	s.assertList(c, stor, "", nil)

	// RemoveAll does not remove the base storage directory.
	_, err = os.Stat(contentDir)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestURL(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	url, err := stor.URL("a/b")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "sftp://example.com/"+path.Join(storageDir, contentdir, "a/b"))
}

func (s *storageSuite) TestConsistencyStrategy(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	c.Assert(stor.DefaultConsistencyStrategy(), gc.Equals, utils.AttemptStrategy{})
}

const defaultFlockTimeout = 5 * time.Second

// flock is a test helper that flocks a file, executes "sleep" with the
// specified duration, the command is terminated in the test tear down.
func (s *storageSuite) flock(c *gc.C, mode flockmode, lockfile string) {
	sleepcmd := fmt.Sprintf("echo started && sleep %vs", defaultFlockTimeout.Seconds())
	cmd := exec.Command(flockBin, "--nonblock", "--close", string(mode), lockfile, "-c", sleepcmd)
	stdout, err := cmd.StdoutPipe()
	c.Assert(err, gc.IsNil)
	c.Assert(cmd.Start(), gc.IsNil)
	// Make sure the flock has been taken before returning by reading stdout waiting for "started"
	_, err = io.ReadFull(stdout, make([]byte, len("started")))
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(*gc.C) {
		cmd.Process.Kill()
		cmd.Process.Wait()
	})
}

func (s *storageSuite) TestCreateFailsIfFlockNotAvailable(c *gc.C) {
	storageDir := c.MkDir()
	s.flock(c, flockShared, storageDir)
	// Creating storage requires an exclusive lock initially.
	//
	// flock exits with exit code 1 if it can't acquire the
	// lock immediately in non-blocking mode (which the tests force).
	_, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.ErrorMatches, "exit code 1")
}

func (s *storageSuite) TestWithSharedLocks(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)

	// Get and List should be able to proceed with a shared lock.
	// All other methods should fail.
	data := []byte("abc\000def")
	c.Assert(ioutil.WriteFile(filepath.Join(storageDir, contentdir, "a"), data, 0644), gc.IsNil)

	s.flock(c, flockShared, storageDir)
	_, err = storage.Get(stor, "a")
	c.Assert(err, gc.IsNil)
	_, err = storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Assert(stor.Put("a", bytes.NewBuffer(nil), 0), gc.NotNil)
	c.Assert(stor.Remove("a"), gc.NotNil)
	c.Assert(stor.RemoveAll(), gc.NotNil)
}

func (s *storageSuite) TestWithExclusiveLocks(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := NewSSHStorage("example.com", storageDir)
	c.Assert(err, gc.IsNil)
	// None of the methods (apart from URL) should be able to do anything
	// while an exclusive lock is held.
	s.flock(c, flockExclusive, storageDir)
	_, err = stor.URL("a")
	c.Assert(err, gc.IsNil)
	c.Assert(stor.Put("a", bytes.NewBuffer(nil), 0), gc.NotNil)
	c.Assert(stor.Remove("a"), gc.NotNil)
	c.Assert(stor.RemoveAll(), gc.NotNil)
	_, err = storage.Get(stor, "a")
	c.Assert(err, gc.NotNil)
	_, err = storage.List(stor, "")
	c.Assert(err, gc.NotNil)
}
