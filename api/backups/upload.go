// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/state/backups"
)

var sshUpload = func(addr string, archive io.Reader) (string, error) {
	return backups.SSHUpload(addr, archive)
}

// Upload sends the backup archive to the server when it is stored.
// The ID by which the stored archive can be found is returned.
func (c *Client) Upload(archive io.Reader) (string, error) {
	// TODO(ericsnow) As a temporary solution for restore we are using
	// an SSH-based upload implementation.  This will be replaced by
	// an HTTP-based solution.
	id, err := sshUpload(c.publicAddress, archive)
	return id, errors.Trace(err)
}
