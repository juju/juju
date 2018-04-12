// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package provides helpers for testing with resources.
package resourcetesting

import (
	"io"
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6/resource"

	"github.com/juju/juju/resource"
)

// NewResource produces full resource info for the given name and
// content. The origin is set set to "upload". A reader is also returned
// which contains the content.
func NewResource(c *gc.C, stub *testing.Stub, name, applicationID, content string) resource.Opened {
	username := "a-user"
	return resource.Opened{
		Resource:   newResource(c, name, applicationID, username, content),
		ReadCloser: newStubReadCloser(stub, content),
	}
}

// NewCharmResource produces basic resource info for the given name
// and content. The origin is set set to "upload".
func NewCharmResource(c *gc.C, name, content string) charmresource.Resource {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        name,
			Type:        charmresource.TypeFile,
			Path:        name + ".tgz",
			Description: name + " description",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    0,
		Fingerprint: fp,
		Size:        int64(len(content)),
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	return res
}

// NewPlaceholderResource returns resource info for a resource that
// has not been uploaded or pulled from the charm store yet. The origin
// is set to "upload".
func NewPlaceholderResource(c *gc.C, name, applicationID string) resource.Resource {
	res := newResource(c, name, applicationID, "", "")
	res.Fingerprint = charmresource.Fingerprint{}
	return res
}

func newResource(c *gc.C, name, applicationID, username, content string) resource.Resource {
	var timestamp time.Time
	if username != "" {
		// TODO(perrito666) 2016-05-02 lp:1558657
		timestamp = time.Now().UTC()
	}
	res := resource.Resource{
		Resource:      NewCharmResource(c, name, content),
		ID:            applicationID + "/" + name,
		PendingID:     "",
		ApplicationID: applicationID,
		Username:      username,
		Timestamp:     timestamp,
	}
	err := res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}

type stubReadCloser struct {
	io.Reader
	io.Closer
}

func newStubReadCloser(stub *testing.Stub, content string) io.ReadCloser {
	return &stubReadCloser{
		Reader: filetesting.NewStubReader(stub, content),
		Closer: &filetesting.StubCloser{
			Stub: stub,
		},
	}
}
