// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/errors"
)

// Session is the interface that wraps the GetObject method.
type Session interface {
	// GetObject returns a reader for the specified object.
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, int64, string, error)
}

// Blobs is the client desienged to interact with the
// s3 compatible object store hosted by the apiserver
type Blobs struct {
	session Session
}

// NewBlobsS3Client creates a client to interact with charm blobs stored
// on the apiserver's s3 compatible object store.
func NewBlobsS3Client(session Session) *Blobs {
	return &Blobs{session: session}
}

// GetCharm retrieves a charm from the S3-compatible object store hosted
// by the apiserver. Returns an archived charm as a stream of bytes
func (c *Blobs) GetCharm(ctx context.Context, modelUUID, charmRef string) (io.ReadCloser, error) {
	bucketName := fmt.Sprintf("model-%s", modelUUID)
	objectName := fmt.Sprintf("charms/%s", charmRef)
	reader, _, _, err := c.session.GetObject(ctx, bucketName, objectName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return reader, nil
}

// GetObject retrieves an object from the S3-compatible object store hosted
// by the apiserver. Returns an archived charm as a stream of bytes
func (c *Blobs) GetObject(ctx context.Context, modelUUID, sha256 string) (io.ReadCloser, int64, error) {
	bucketName := fmt.Sprintf("model-%s", modelUUID)
	objectName := fmt.Sprintf("objects/%s", sha256)
	reader, size, _, err := c.session.GetObject(ctx, bucketName, objectName)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	return reader, size, nil
}
