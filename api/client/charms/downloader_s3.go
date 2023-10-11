// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"io"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/downloader"
)

// NewS3CharmDownloader returns a new charm downloader that wraps a s3Caller
// client for the provided endpoint.
func NewS3CharmDownloader(objectStoreClient objectstore.Session, apiCaller base.APICaller) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(req downloader.Request) (io.ReadCloser, error) {
			streamer := NewS3CharmOpener(objectStoreClient, apiCaller)
			reader, err := streamer.OpenCharm(req)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return reader, nil
		},
	}
	return dlr
}

// CharmOpener provides the OpenCharm method.
type S3CharmOpener interface {
	OpenCharm(req downloader.Request) (io.ReadCloser, error)
}

type s3charmOpener struct {
	ctx               context.Context
	objectStoreCaller objectstore.Session
	apiCaller         base.APICaller
}

func (s *s3charmOpener) OpenCharm(req downloader.Request) (io.ReadCloser, error) {
	// Retrieve first 8 characters of the charm archive sha256
	if len(req.ArchiveSha256) < 8 {
		return nil, errors.NotValidf("download request with archiveSha256 length %d", len(req.ArchiveSha256))
	}
	shortSha256 := req.ArchiveSha256[0:8]
	// Retrieve charms name
	curl, err := charm.ParseURL(req.URL.String())
	if err != nil {
		return nil, errors.Annotate(err, "did not receive a valid charm URL")
	}

	// We can ignore the second return bool from ModelTag() because if it's
	// a controller model, then it will fail later when calling GetObject.
	modelTag, _ := s.apiCaller.ModelTag()
	bucket := "model-" + modelTag.Id()

	object := "charms/" + curl.Name + "-" + shortSha256
	return s.objectStoreCaller.GetObject(s.ctx, bucket, object)
}

// NewS3CharmOpener returns a charm opener for the specified s3Caller.
func NewS3CharmOpener(objectStoreCaller objectstore.Session, apiCaller base.APICaller) S3CharmOpener {
	return &s3charmOpener{
		ctx:               context.Background(),
		objectStoreCaller: objectStoreCaller,
		apiCaller:         apiCaller,
	}
}
