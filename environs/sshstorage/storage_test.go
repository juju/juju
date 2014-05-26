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
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/storage"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

type storageSuite struct {
	coretesting.BaseSuite
	bin string
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) sshCommand(c *gc.C, host string, command ...string) *ssh.Cmd {
	script := []byte("#!/bin/bash\n" + strings.Join(command, " "))
	err := ioutil.WriteFile(filepath.Join(s.bin, "ssh"), script, 0755)
	c.Assert(err, gc.IsNil)
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, gc.IsNil)
	return client.Command(host, command, nil)
}

func newSSHStorage(host, storageDir, tmpDir string) (*SSHStorage, error) {
	params := NewSSHStorageParams{
		Host:       host,
		StorageDir: storageDir,
		TmpDir:     tmpDir,
	}
	return NewSSHStorage(params)
}

// flockBin is the path to the original "flock" binary.
var flockBin string

func (s *storageSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	var err error
	flockBin, err = exec.LookPath("flock")
	c.Assert(err, gc.IsNil)

	s.bin = c.MkDir()
	s.PatchEnvPathPrepend(s.bin)

	// Create a "sudo" command which shifts away the "-n", sets
	// SUDO_UID/SUDO_GID, and executes the remaining args.
	err = ioutil.WriteFile(filepath.Join(s.bin, "sudo"), []byte(
		"#!/bin/sh\nshift; export SUDO_UID=`id -u` SUDO_GID=`id -g`; exec \"$@\"",
	), 0755)
	c.Assert(err, gc.IsNil)
	restoreSshCommand := testing.PatchValue(&sshCommand, func(host string, command ...string) *ssh.Cmd {
		return s.sshCommand(c, host, command...)
	})
	s.AddSuiteCleanup(func(*gc.C) { restoreSshCommand() })

	// Create a new "flock" which calls the original, but in non-blocking mode.
	data := []byte(fmt.Sprintf("#!/bin/sh\nexec %s --nonblock \"$@\"", flockBin))
	err = ioutil.WriteFile(filepath.Join(s.bin, "flock"), data, 0755)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) makeStorage(c *gc.C) (storage *SSHStorage, storageDir string) {
	storageDir = c.MkDir()
	storage, err := newSSHStorage("example.com", storageDir, storageDir+"-tmp")
	c.Assert(err, gc.IsNil)
	c.Assert(storage, gc.NotNil)
	s.AddCleanup(func(*gc.C) { storage.Close() })
	return storage, storageDir
}

// createFiles creates empty files in the storage directory
// with the given storage names.
func createFiles(c *gc.C, storageDir string, names ...string) {
	for _, name := range names {
		path := filepath.Join(storageDir, filepath.FromSlash(name))
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			c.Assert(err, jc.Satisfies, os.IsExist)
		}
		err := ioutil.WriteFile(path, nil, 0644)
		c.Assert(err, gc.IsNil)
	}
}

func (s *storageSuite) TestnewSSHStorage(c *gc.C) {
	storageDir := c.MkDir()
	// Run this block twice to ensure newSSHStorage can reuse
	// an existing storage location.
	for i := 0; i < 2; i++ {
		stor, err := newSSHStorage("example.com", storageDir, storageDir+"-tmp")
		c.Assert(err, gc.IsNil)
		c.Assert(stor, gc.NotNil)
		c.Assert(stor.Close(), gc.IsNil)
	}
	err := os.RemoveAll(storageDir)
	c.Assert(err, gc.IsNil)

	// You must have permissions to create the directory.
	storageDir = c.MkDir()
	err = os.Chmod(storageDir, 0555)
	c.Assert(err, gc.IsNil)
	_, err = newSSHStorage("example.com", filepath.Join(storageDir, "subdir"), storageDir+"-tmp")
	c.Assert(err, gc.ErrorMatches, "(.|\n)*cannot change owner and permissions of(.|\n)*")
}

func (s *storageSuite) TestPathValidity(c *gc.C) {
	stor, storageDir := s.makeStorage(c)
	err := os.Mkdir(filepath.Join(storageDir, "a"), 0755)
	c.Assert(err, gc.IsNil)
	createFiles(c, storageDir, "a/b")

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
	stor, storageDir := s.makeStorage(c)
	data := []byte("abc\000def")
	err := os.Mkdir(filepath.Join(storageDir, "a"), 0755)
	c.Assert(err, gc.IsNil)
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err = ioutil.WriteFile(filepath.Join(storageDir, name), data, 0644)
		c.Assert(err, gc.IsNil)
		r, err := storage.Get(stor, name)
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.DeepEquals, data)
	}
	_, err = storage.Get(stor, "notthere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *storageSuite) TestWriteFailure(c *gc.C) {
	// Invocations:
	//  1: first "install"
	//  2: touch, Put
	//  3: second "install"
	//  4: touch
	var invocations int
	badSshCommand := func(host string, command ...string) *ssh.Cmd {
		invocations++
		switch invocations {
		case 1, 3:
			return s.sshCommand(c, host, "true")
		case 2:
			// Note: must close stdin before responding the first time, or
			// the second command will race with closing stdin, and may
			// flush first.
			return s.sshCommand(c, host, "head -n 1 > /dev/null; exec 0<&-; echo JUJU-RC: 0; echo blah blah; echo more")
		case 4:
			return s.sshCommand(c, host, `head -n 1 > /dev/null; echo "Hey it's JUJU-RC: , but not at the beginning of the line"; echo more`)
		default:
			c.Errorf("unexpected invocation: #%d, %s", invocations, command)
			return nil
		}
	}
	s.PatchValue(&sshCommand, badSshCommand)

	stor, err := newSSHStorage("example.com", c.MkDir(), c.MkDir())
	c.Assert(err, gc.IsNil)
	defer stor.Close()
	err = stor.Put("whatever", bytes.NewBuffer(nil), 0)
	c.Assert(err, gc.ErrorMatches, `failed to write input: write \|1: broken pipe \(output: "blah blah\\nmore"\)`)

	_, err = newSSHStorage("example.com", c.MkDir(), c.MkDir())
	c.Assert(err, gc.ErrorMatches, `failed to locate "JUJU-RC: " \(output: "Hey it's JUJU-RC: , but not at the beginning of the line\\nmore"\)`)
}

func (s *storageSuite) TestPut(c *gc.C) {
	stor, storageDir := s.makeStorage(c)
	data := []byte("abc\000def")
	for _, name := range []string{"b", filepath.Join("a", "b")} {
		err := stor.Put(name, bytes.NewBuffer(data), int64(len(data)))
		c.Assert(err, gc.IsNil)
		out, err := ioutil.ReadFile(filepath.Join(storageDir, name))
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
	stor, storageDir := s.makeStorage(c)
	s.assertList(c, stor, "", nil)

	// Directories don't show up in List.
	err := os.Mkdir(filepath.Join(storageDir, "a"), 0755)
	c.Assert(err, gc.IsNil)
	s.assertList(c, stor, "", nil)
	s.assertList(c, stor, "a", nil)
	createFiles(c, storageDir, "a/b1", "a/b2", "b")
	s.assertList(c, stor, "", []string{"a/b1", "a/b2", "b"})
	s.assertList(c, stor, "a", []string{"a/b1", "a/b2"})
	s.assertList(c, stor, "a/b", []string{"a/b1", "a/b2"})
	s.assertList(c, stor, "a/b1", []string{"a/b1"})
	s.assertList(c, stor, "a/b3", nil)
	s.assertList(c, stor, "a/b/c", nil)
	s.assertList(c, stor, "b", []string{"b"})
}

func (s *storageSuite) TestRemove(c *gc.C) {
	stor, storageDir := s.makeStorage(c)
	err := os.Mkdir(filepath.Join(storageDir, "a"), 0755)
	c.Assert(err, gc.IsNil)
	createFiles(c, storageDir, "a/b1", "a/b2")
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
	stor, storageDir := s.makeStorage(c)
	err := os.Mkdir(filepath.Join(storageDir, "a"), 0755)
	c.Assert(err, gc.IsNil)
	createFiles(c, storageDir, "a/b1", "a/b2")
	s.assertList(c, stor, "", []string{"a/b1", "a/b2"})
	c.Assert(stor.RemoveAll(), gc.IsNil)
	s.assertList(c, stor, "", nil)

	// RemoveAll does not remove the base storage directory.
	_, err = os.Stat(storageDir)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestURL(c *gc.C) {
	stor, storageDir := s.makeStorage(c)
	url, err := stor.URL("a/b")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "sftp://example.com/"+path.Join(storageDir, "a/b"))
}

func (s *storageSuite) TestDefaultConsistencyStrategy(c *gc.C) {
	stor, _ := s.makeStorage(c)
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
	_, err := newSSHStorage("example.com", storageDir, storageDir+"-tmp")
	c.Assert(err, gc.ErrorMatches, "exit code 1")
}

func (s *storageSuite) TestWithSharedLocks(c *gc.C) {
	stor, storageDir := s.makeStorage(c)

	// Get and List should be able to proceed with a shared lock.
	// All other methods should fail.
	createFiles(c, storageDir, "a")

	s.flock(c, flockShared, storageDir)
	_, err := storage.Get(stor, "a")
	c.Assert(err, gc.IsNil)
	_, err = storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Assert(stor.Put("a", bytes.NewBuffer(nil), 0), gc.NotNil)
	c.Assert(stor.Remove("a"), gc.NotNil)
	c.Assert(stor.RemoveAll(), gc.NotNil)
}

func (s *storageSuite) TestWithExclusiveLocks(c *gc.C) {
	stor, storageDir := s.makeStorage(c)
	// None of the methods (apart from URL) should be able to do anything
	// while an exclusive lock is held.
	s.flock(c, flockExclusive, storageDir)
	_, err := stor.URL("a")
	c.Assert(err, gc.IsNil)
	c.Assert(stor.Put("a", bytes.NewBuffer(nil), 0), gc.NotNil)
	c.Assert(stor.Remove("a"), gc.NotNil)
	c.Assert(stor.RemoveAll(), gc.NotNil)
	_, err = storage.Get(stor, "a")
	c.Assert(err, gc.NotNil)
	_, err = storage.List(stor, "")
	c.Assert(err, gc.NotNil)
}

func (s *storageSuite) TestPutLarge(c *gc.C) {
	stor, _ := s.makeStorage(c)
	buf := make([]byte, 1048576)
	err := stor.Put("ohmy", bytes.NewBuffer(buf), int64(len(buf)))
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestStorageDirBlank(c *gc.C) {
	tmpdir := c.MkDir()
	_, err := newSSHStorage("example.com", "", tmpdir)
	c.Assert(err, gc.ErrorMatches, "storagedir must be specified and non-empty")
}

func (s *storageSuite) TestTmpDirBlank(c *gc.C) {
	storageDir := c.MkDir()
	_, err := newSSHStorage("example.com", storageDir, "")
	c.Assert(err, gc.ErrorMatches, "tmpdir must be specified and non-empty")
}

func (s *storageSuite) TestTmpDirExists(c *gc.C) {
	// If we explicitly set the temporary directory,
	// it may already exist, but doesn't have to.
	storageDir := c.MkDir()
	tmpdirs := []string{storageDir, filepath.Join(storageDir, "subdir")}
	for _, tmpdir := range tmpdirs {
		stor, err := newSSHStorage("example.com", storageDir, tmpdir)
		defer stor.Close()
		c.Assert(err, gc.IsNil)
		err = stor.Put("test-write", bytes.NewReader(nil), 0)
		c.Assert(err, gc.IsNil)
	}
}

func (s *storageSuite) TestTmpDirPermissions(c *gc.C) {
	// newSSHStorage will fail if it can't create or change the
	// permissions of the temporary directory.
	storageDir := c.MkDir()
	tmpdir := c.MkDir()
	os.Chmod(tmpdir, 0400)
	defer os.Chmod(tmpdir, 0755)
	_, err := newSSHStorage("example.com", storageDir, filepath.Join(tmpdir, "subdir2"))
	c.Assert(err, gc.ErrorMatches, ".*install: cannot create directory.*Permission denied.*")
}

func (s *storageSuite) TestPathCharacters(c *gc.C) {
	storageDirBase := c.MkDir()
	storageDir := filepath.Join(storageDirBase, "'")
	tmpdir := filepath.Join(storageDirBase, `"`)
	c.Assert(os.Mkdir(storageDir, 0755), gc.IsNil)
	c.Assert(os.Mkdir(tmpdir, 0755), gc.IsNil)
	_, err := newSSHStorage("example.com", storageDir, tmpdir)
	c.Assert(err, gc.IsNil)
}
