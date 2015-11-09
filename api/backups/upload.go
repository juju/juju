// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"net/http"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/httpattachment"
	"github.com/juju/juju/apiserver/params"
)

// Upload sends the backup archive to remote storage.
func (c *Client) Upload(archive io.ReadSeeker, meta params.BackupsMetadataResult) (string, error) {
	// Empty out some of the metadata.
	meta.ID = ""
	meta.Stored = time.Time{}

	req, err := http.NewRequest("PUT", "/backups", nil)
	if err != nil {
		return "", errors.Trace(err)
	}
	body, contentType, err := httpattachment.NewBody(archive, meta, "juju-backup.tar.gz")
	if err != nil {
		return "", errors.Annotatef(err, "cannot create multipart body")
	}
	req.Header.Set("Content-Type", contentType)
	var result params.BackupsMetadataResult
	if err := c.client.Do(req, body, &result); err != nil {
		return "", errors.Trace(err)
	}
	return result.ID, nil
}
