// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongodoc // import "gopkg.in/juju/charmstore.v5/internal/mongodoc"

import (
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
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

	// UploadTime is the is the time the resource file was stored in
	// the blob store.
	UploadTime time.Time

	// For Kubernetes charms holds the SHA256 digest of the image.
	// If this is set, none of the fields after DockerImageName will be set.
	DockerImageDigest string `bson:",omitempty"`

	// DockerImageName holds the name of the docker image if it's
	// external to the charm store's docker registry.
	DockerImageName string `bson:",omitempty"`

	// BlobHash holds the hash checksum of the blob, in hexadecimal format,
	// as created by blobstore.NewHash.
	BlobHash string `bson:",omitempty"`

	// Size is the size of the resource file, in bytes.
	Size int64 `bson:"size,omitempty"`

	// BlobIndex stores the multipart index when the blob
	// is composed of several parts.
	BlobIndex *MultipartIndex `bson:",omitempty"`
}

// MultipartIndex holds the index of all the parts of a multipart blob.
type MultipartIndex struct {
	Sizes  []uint32
	Hashes Hashes
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

	if doc.DockerImageDigest == "" {
		if doc.BlobHash == "" {
			return errgo.New("missing blob hash")
		}
		if doc.Size < 0 {
			return errgo.Newf("got negative size %d", doc.Size)
		}
	} else {
		if doc.BlobHash != "" {
			return errgo.New("cannot combine docker image digest with blob hash")
		}
		if doc.Size != 0 {
			return errgo.Newf("cannot combine docker image size with blob hash")
		}
	}

	if doc.UploadTime.IsZero() {
		return errgo.New("missing upload timestamp")
	}

	return nil
}
