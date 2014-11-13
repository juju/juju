// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/utils/ssh"
)

const sshUser = "ubuntu"

var sshCopyReader = func(host, filename string, archive io.Reader) error {
	return ssh.CopyReader(host, filename, archive, nil)
}

// Upload sends the backup archive to the server when it is stored.
// The ID by which the stored archive can be found is returned.
func (c *Client) Upload(archive io.Reader) (string, error) {
	// TODO(ericsnow) sshCopyReader assumes the proper SSH keys are
	// already in place (which they will be when the client is
	// initiated from the CLI).  However, this SSH-based implementation
	// is a temporary solution that will be replaced by an HTTP-based
	// one (which won't have any problem with keys).

	// We upload the file to the user's home directory.
	filename := time.Now().UTC().Format("juju-backup-20060102-150405.tgz")
	host := sshUser + "@" + c.publicAddress
	err := sshCopyReader(host, filename, archive)
	return "file://" + filename, errors.Trace(err)
}
