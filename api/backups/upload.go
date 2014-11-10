// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/utils/ssh"
)

func dumpFile(file io.Reader) (string, error) {
	tempfile, err := ioutil.TempFile(os.TempDir(), "juju-backup-")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer tempfile.Close()

	_, err := io.Copy(tempfile, file)
	if err != nil {
		return "", errors.Trace(err)
	}

	return tempfile.Name(), nil
}

// Upload sends the backup archive to the server when it is stored.
// The ID by which the stored archive can be found is returned.
func (c *Client) Upload(archive io.Reader) (string, error) {
	addr, err := c.publicAddress()
	if err != nil {
		return "", errors.Trace(err)
	}

	filename, err := dumpFile(archive)
	if err != nil {
		return "", errors.Trace(err)
	}

	remote := fmt.Sprintf("ubuntu@%s:%s", addr, filename)
	err = ssh.Copy([]string{filename, remote}, nil)
	return "file://" + filename, errors.Trace(err)
}
