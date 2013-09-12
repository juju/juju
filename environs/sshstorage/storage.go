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
func NewSSHStorage(host string, remotepath string) (*SSHStorage, error) {
	script := fmt.Sprintf("install -d -g $SUDO_GID -o $SUDO_UID %s", remotepath)
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
		remotepath: remotepath,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		scanner:    bufio.NewScanner(stdout),
	}
	cmd.Start()

	// Verify We have write permissions.
	if _, err = storage.runf(flockExclusive, "touch %s", utils.ShQuote(remotepath)); err != nil {
		stdin.Close()
		stdout.Close()
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
	command = fmt.Sprintf("(%s) 2>&1; echo %s$?", command, rcPrefix)
	command = fmt.Sprintf("flock %s %s -c %s", flockmode, s.remotepath, utils.ShQuote(command))
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
		if strings.Contains(err.Output, "No such file or directory") {
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
	return fmt.Sprintf("sftp://%s/%s/%s", s.host, s.remotepath, name), nil
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
	_, err = s.runf(flockExclusive, "mkdir -p `dirname %s` && base64 -d > %s << EOF\n%s\nEOF\n", path, path, encoded)
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
