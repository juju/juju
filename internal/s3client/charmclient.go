// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"fmt"
	"io"
)

// Charms is the client desienged to interact with the
// s3 compatible object store hosted by the apiserver
type Charms struct {
	session Session
}

// GetCharm retrieves a charm from the S3-compatible object store hosted
// by the apiserver. Returns an archived charm as a stream of bytes
func (c *Charms) GetCharm(ctx context.Context, modelUUID, charmRef string) (io.ReadCloser, error) {
	bucketName := fmt.Sprintf("model-%s", modelUUID)
	objectName := fmt.Sprintf("charms/%s", charmRef)
	return c.session.GetObject(ctx, bucketName, objectName)
}

// NewCharmsS3Client creates a client to interact with charm blobs stored
// on the apiserver's s3 comptabile object store.
func NewCharmsS3Client(session Session) *Charms {
	return &Charms{session: session}
}
