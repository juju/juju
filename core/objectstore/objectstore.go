// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"
)

// Session provides access to the object store.
type Session interface {
	// GetObject returns a reader for the specified object.
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
}
