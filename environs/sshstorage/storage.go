// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
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

// SSHStorage implements storage.Storage.
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

// NewSSHStorage creates a new SSHStorage, connected to the
// specified host, managing state under the specified remote path.
//
// A temporary directory must be specified, and should be located on the
// same filesystem as the storage directory to ensure atomic writes.
// The temporary directory will be created when NewSSHStorage is invoked
// if it doesn't already exist; it will never be removed. NewSSHStorage
// will attempt to reassign ownership to the login user, and will return
// an error if it cannot do so.
func NewSSHStorage(host, storagedir, tmpdir string) (*SSHStorage, error) {
	if storagedir == "" {
		return nil, errors.New("storagedir must be specified and non-empty")
	}
	if tmpdir == "" {
		return nil, errors.New("tmpdir must be specified and non-empty")
	}

	script := fmt.Sprintf(
		"install -d -g $SUDO_GID -o $SUDO_UID %s %s",
		utils.ShQuote(storagedir),
		utils.ShQuote(tmpdir),
	)

	cmd := sshCommand(host, true, fmt.Sprintf("sudo bash -c %s", utils.ShQuote(script)))
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
	stor := &SSHStorage{
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
	_, err = stor.runf(flockExclusive, "touch %s", utils.ShQuote(storagedir))
	if err != nil {
		stdin.Close()
		stdout.Close()
		cmd.Wait()
		return nil, err
	}
	return stor, nil
}

// Close cleanly terminates the underlying SSH connection.
func (s *SSHStorage) Close() error {
	s.stdin.Close()
	s.stdout.Close()
	return s.cmd.Wait()
}

func (s *SSHStorage) runf(flockmode flockmode, command string, args ...interface{}) (string, error) {
	command = fmt.Sprintf(command, args...)
	return s.run(flockmode, command, nil)
}

func (s *SSHStorage) run(flockmode flockmode, command string, input []byte) (string, error) {
	const rcPrefix = "JUJU-RC: "
	command = fmt.Sprintf(
		"SHELL=/bin/bash flock %s %s -c %s",
		flockmode,
		s.remotepath,
		utils.ShQuote(command),
	)
	if input != nil {
		command = fmt.Sprintf("line | base64 -d | (%s)", command)
	}
	command = fmt.Sprintf("(%s) 2>&1; echo %s$?", command, rcPrefix)
	if _, err := s.stdin.Write([]byte(command + "\n")); err != nil {
		return "", fmt.Errorf("failed to write command: %v", err)
	}
	if input != nil {
		encoded := base64.StdEncoding.EncodeToString(input)
		if _, err := s.stdin.Write([]byte(encoded + "\n")); err != nil {
			return "", fmt.Errorf("failed to write input: %v", err)
		}
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

// Get implements storage.StorageReader.Get.
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

// List implements storage.StorageReader.List.
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

// URL implements storage.StorageReader.URL.
func (s *SSHStorage) URL(name string) (string, error) {
	path, err := s.path(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sftp://%s/%s", s.host, path), nil
}

// DefaultConsistencyStrategy implements storage.StorageReader.ConsistencyStrategy.
func (s *SSHStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *SSHStorage) ShouldRetry(err error) bool {
	return false
}

// Put implements storage.StorageWriter.Put
func (s *SSHStorage) Put(name string, r io.Reader, length int64) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	buf := make([]byte, length)
	if _, err := r.Read(buf); err != nil {
		return err
	}
	path = utils.ShQuote(path)
	tmpdir := utils.ShQuote(s.tmpdir)

	// Write to a temporary file ($TMPFILE), then mv atomically.
	command := fmt.Sprintf("mkdir -p `dirname %s` && cat > $TMPFILE", path)
	command = fmt.Sprintf(
		"export TMPDIR=%s && TMPFILE=`mktemp` && ((%s && mv $TMPFILE %s) || rm -f $TMPFILE)",
		tmpdir, command, path,
	)

	_, err = s.run(flockExclusive, command+"\n", buf)
	return err
}

// Remove implements storage.StorageWriter.Remove
func (s *SSHStorage) Remove(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	path = utils.ShQuote(path)
	_, err = s.runf(flockExclusive, "rm -f %s", path)
	return err
}

// RemoveAll implements storage.StorageWriter.RemoveAll
func (s *SSHStorage) RemoveAll() error {
	_, err := s.runf(flockExclusive, "rm -fr %s/*", utils.ShQuote(s.remotepath))
	return err
}
