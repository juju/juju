// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

// SSHStorage implements environs.Storage.
//
// The storage is created under sudo, and ownership given over to the
// login uid/gid. This is done so that we don't require sudo, and by
// consequence, don't require a pty, so we can interact with a script
// via stdin.
type SSHStorage struct {
	host       string
	remotepath string
	tmpdir     string

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
}

var sshCommand = func(host string, tty bool, command string) *exec.Cmd {
	sshArgs := []string{host}
	if tty {
		sshArgs = append(sshArgs, "-t")
	}
	sshArgs = append(sshArgs, "--", command)
	return exec.Command("ssh", sshArgs...)
}

type flockmode string

const (
	flockShared    flockmode = "-s"
	flockExclusive flockmode = "-x"
)

// UseDefaultTmpDir may be passed into NewSSHStorage
// for the tmpdir argument, to signify that the default
// value should be used. See NewSSHStorage for more.
const UseDefaultTmpDir = ""

// NewSSHStorage creates a new SSHStorage, connected to the
// specified host, managing state under the specified remote path.
//
// A temporary directory may be specified, in which case it should
// be a directory on the same filesystem as the storage directory
// to ensure atomic writes. If left unspecified, tmpdir will be
// assigned a value of storagedir+".tmp".
//
// If tmpdir == UseDefaultTmpDir, it will be created when Put is invoked,
// and will be removed afterwards. If tmpdir != UseDefaultTmpDir, it must
// already exist, and will never be removed.
func NewSSHStorage(host, storagedir, tmpdir string) (*SSHStorage, error) {
	script := fmt.Sprintf("install -d -g $SUDO_GID -o $SUDO_UID %s", utils.ShQuote(storagedir))
	cmd := sshCommand(host, true, fmt.Sprintf("sudo bash -c '%s'", script))
	cmd.Stdin = os.Stdin
	if out, err := cmd.CombinedOutput(); err != nil {
		err = fmt.Errorf("failed to create storage dir: %v (%v)", err, strings.TrimSpace(string(out)))
		return nil, err
	}

	// We could use sftp, but then we'd be at the mercy of
	// sftp's output messages for checking errors. Instead,
	// we execute an interactive bash shell.
	cmd = sshCommand(host, false, "bash")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	storage := &SSHStorage{
		host:       host,
		remotepath: storagedir,
		tmpdir:     tmpdir,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		scanner:    bufio.NewScanner(stdout),
	}
	cmd.Start()

	// Verify we have write permissions.
	_, err = storage.runf(flockExclusive, "touch %s", utils.ShQuote(storagedir))
	if err != nil {
		stdin.Close()
		stdout.Close()
		cmd.Wait()
		return nil, err
	}
	return storage, nil
}

// Close cleanly terminates the underlying SSH connection.
func (s *SSHStorage) Close() error {
	s.stdin.Close()
	s.stdout.Close()
	return s.cmd.Wait()
}

func (s *SSHStorage) runf(flockmode flockmode, command string, args ...interface{}) (string, error) {
	command = fmt.Sprintf(command, args...)
	return s.run(flockmode, command)
}

func (s *SSHStorage) run(flockmode flockmode, command string) (string, error) {
	const rcPrefix = "JUJU-RC: "
	command = fmt.Sprintf(
		"(SHELL=/bin/bash flock %s %s -c %s) 2>&1; echo %s$?",
		flockmode,
		s.remotepath,
		utils.ShQuote(command),
		rcPrefix,
	)
	if _, err := s.stdin.Write([]byte(command + "\r\n")); err != nil {
		return "", fmt.Errorf("failed to write command: %v", err)
	}
	var output []string
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if strings.HasPrefix(line, rcPrefix) {
			line := line[len(rcPrefix):]
			rc, err := strconv.Atoi(line)
			if err != nil {
				return "", fmt.Errorf("failed to parse exit code %q: %v", line, err)
			}
			outputJoined := strings.Join(output, "\n")
			if rc == 0 {
				return outputJoined, nil
			}
			return "", SSHStorageError{outputJoined, rc}
		} else {
			output = append(output, line)
		}
	}
	return "", s.scanner.Err()
}

// path returns a remote absolute path for a storage object name.
func (s *SSHStorage) path(name string) (string, error) {
	remotepath := path.Clean(path.Join(s.remotepath, name))
	if !strings.HasPrefix(remotepath, s.remotepath) {
		return "", fmt.Errorf("%q escapes storage directory", name)
	}
	return remotepath, nil
}

// Get implements environs.StorageReader.Get.
func (s *SSHStorage) Get(name string) (io.ReadCloser, error) {
	path, err := s.path(name)
	if err != nil {
		return nil, err
	}
	out, err := s.runf(flockShared, "base64 < %s", utils.ShQuote(path))
	if err != nil {
		err := err.(SSHStorageError)
		if strings.Contains(err.Output, "No such file") {
			return nil, coreerrors.NewNotFoundError(err, "")
		}
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewBuffer(decoded)), nil
}

// List implements environs.StorageReader.List.
func (s *SSHStorage) List(prefix string) ([]string, error) {
	remotepath, err := s.path(prefix)
	if err != nil {
		return nil, err
	}
	dir, prefix := path.Split(remotepath)
	quotedDir := utils.ShQuote(dir)
	out, err := s.runf(flockShared, "(test -d %s && find %s -type f) || true", quotedDir, quotedDir)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var names []string
	for _, name := range strings.Split(out, "\n") {
		if strings.HasPrefix(name[len(dir):], prefix) {
			names = append(names, name[len(s.remotepath)+1:])
		}
	}
	sort.Strings(names)
	return names, nil
}

// URL implements environs.StorageReader.URL.
func (s *SSHStorage) URL(name string) (string, error) {
	path, err := s.path(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sftp://%s/%s", s.host, path), nil
}

// ConsistencyStrategy implements environs.StorageReader.ConsistencyStrategy.
func (s *SSHStorage) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// Put implements environs.StorageWriter.Put
func (s *SSHStorage) Put(name string, r io.Reader, length int64) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	buf := make([]byte, length)
	if _, err := r.Read(buf); err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(buf)
	path = utils.ShQuote(path)

	tmpdir := s.tmpdir
	if tmpdir == UseDefaultTmpDir {
		tmpdir = s.remotepath + ".tmp"
	}
	tmpdir = utils.ShQuote(tmpdir)

	// Write to a temporary file ($TMPFILE), then mv atomically.
	command := fmt.Sprintf("mkdir -p `dirname %s` && base64 -d > $TMPFILE", path)
	command = fmt.Sprintf(
		"export TMPDIR=%s && TMPFILE=`mktemp` && ((%s && mv $TMPFILE %s) || rm -f $TMPFILE)",
		tmpdir, command, path,
	)

	// If UseDefaultTmpDir is passed, then create the
	// temporary directory, and remove it afterwards.
	// Otherwise, the temporary directory is expected
	// to already exist, and is never removed.
	if s.tmpdir == UseDefaultTmpDir {
		installTmpdir := fmt.Sprintf("install -d -g $SUDO_GID -o $SUDO_UID %s", tmpdir)
		removeTmpdir := fmt.Sprintf("rm -fr %s", tmpdir)
		command = fmt.Sprintf("%s && (%s); rc=$?; %s; exit $rc", installTmpdir, command, removeTmpdir)
	}

	command = fmt.Sprintf("(%s) << EOF\n%s\nEOF", command, encoded)
	_, err = s.runf(flockExclusive, command+"\n")
	return err
}

// Remove implements environs.StorageWriter.Remove
func (s *SSHStorage) Remove(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	path = utils.ShQuote(path)
	_, err = s.runf(flockExclusive, "rm -f %s", path)
	return err
}

// RemoveAll implements environs.StorageWriter.RemoveAll
func (s *SSHStorage) RemoveAll() error {
	_, err := s.runf(flockExclusive, "rm -fr %s/*", utils.ShQuote(s.remotepath))
	return err
}
