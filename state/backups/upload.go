// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/utils/ssh"
)

const (
	uploadedPrefix   = "file://"
	sshUsername      = "ubuntu"
	uploadedFilename = FilenamePrefix + "20060102-150405.tgz"
)

var sshCopyReader = func(host, filename string, archive io.Reader) error {
	// sshCopyReader assumes the proper SSH keys are already in place.
	return ssh.CopyReader(host, filename, archive, nil)
}

// SSHUpload sends the backup archive to the server where it is saved
// in the home directory of the SSH user.  The returned ID may be used
// to locate the file on the server.
func SSHUpload(publicAddress string, archive io.Reader) (string, error) {
	filename := time.Now().UTC().Format(uploadedFilename)
	host := sshUsername + "@" + publicAddress
	err := sshCopyReader(host, filename, archive)
	return uploadedPrefix + filename, errors.Trace(err)
}

func resolveUploaded(id string) (string, error) {
	filename := strings.TrimPrefix(id, uploadedPrefix)
	filename = filepath.FromSlash(filename)
	if !strings.HasPrefix(filepath.Base(filename), FilenamePrefix) {
		return "", errors.Errorf("invalid ID for uploaded file: %q", id)
	}
	if filepath.IsAbs(filename) {
		return "", errors.Errorf("expected relative path in ID, got %q", id)
	}

	sshUser, err := user.Lookup(sshUsername)
	if err != nil {
		return "", errors.Trace(err)
	}
	filename = filepath.Join(sshUser.HomeDir, filename)
	return filename, nil
}

func openUploaded(id string) (io.ReadCloser, error) {
	filename, err := resolveUploaded(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	archive, err := os.Open(filename)
	return archive, errors.Trace(err)
}
