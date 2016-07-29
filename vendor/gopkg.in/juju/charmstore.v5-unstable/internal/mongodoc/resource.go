// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongodoc

import (
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

// Resource holds the in-database representation of a charm resource at a
// particular revision, The combination of BaseURL, Name and Revision
// provide a unique key for a resource.
type Resource struct {
	// BaseURL identifies the base URL of the charm associated with this
	// resource.
	BaseURL *charm.URL

	// Name is the name of the resource as defined in the charm
	// metadata.
	Name string

	// Revision identifies the specific revision of the resource.
	Revision int

	// BlobHash holds the hash checksum of the blob, in hexadecimal format,
	// as created by blobstore.NewHash.
	BlobHash string

	// Size is the size of the resource file, in bytes.
	Size int64 `bson:"size"`

	// BlobName holds the name that the resource blob is given in the
	// blob store.
	BlobName string

	// UploadTime is the is the time the resource file was stored in
	// the blob store.
	UploadTime time.Time
}

// Validate ensures that the doc is valid.
func (doc *Resource) Validate() error {
	if doc == nil {
		return errgo.New("no document")
	}
	if doc.BaseURL == nil {
		return errgo.New("missing charm URL")
	}
	if doc.BaseURL.Revision != -1 {
		return errgo.Newf("resolved charm URLs not supported (got revision %d)", doc.BaseURL.Revision)
	}
	if doc.BaseURL.Series != "" {
		return errgo.Newf("series should not be set (got %q)", doc.BaseURL.Series)
	}

	if doc.Name == "" {
		return errgo.New("missing name")
	}
	if doc.Revision < 0 {
		return errgo.Newf("got negative revision %d", doc.Revision)
	}

	if doc.BlobHash == "" {
		return errgo.New("missing blob hash")
	}

	if doc.Size < 0 {
		return errgo.Newf("got negative size %d", doc.Size)
	}

	if doc.BlobName == "" {
		return errgo.New("missing blob name")
	}

	if doc.UploadTime.IsZero() {
		return errgo.New("missing upload timestamp")
	}

	return nil
}
