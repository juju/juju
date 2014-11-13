// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/utils/ssh"
)

var sshUpload = func(host, filename string, archive io.Reader) error {
	// We assume the proper SSH keys are already in place.
	return ssh.CopyReader(host, filename, archive, nil)
}

// Upload sends the backup archive to the server where it is stored.
// The ID by which the stored archive can be found is returned.
func (c *Client) Upload(archive io.Reader) (string, error) {
	// TODO(ericsnow) As a temporary solution for restore we are using
	// an SSH-based upload implementation.  This will be replaced by
	// an HTTP-based solution.
	id, err := backups.SimpleUpload(c.publicAddress, archive, sshUpload)
	return id, errors.Trace(err)
}
