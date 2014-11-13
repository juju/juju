// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/utils/ssh"
)

var sshCopyReader = func(host, filename string, archive io.Reader) error {
	return ssh.CopyReader(host, filename, archive, nil)
}

// Upload sends the backup archive to the server when it is stored.
// The ID by which the stored archive can be found is returned.
func (c *Client) Upload(archive io.Reader) (string, error) {
	filename := time.Now().UTC().Format("/tmp/juju-backup-20060102-150405.tgz")
	host := "ubuntu@" + c.publicAddress
	err := sshCopyReader(host, filename, archive)
	return "file://" + filename, errors.Trace(err)
}
