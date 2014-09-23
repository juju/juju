// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"testing"

	"github.com/juju/errors"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/metadata"
	coretesting "github.com/juju/juju/testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type fakeBackups struct {
	meta    *metadata.Metadata
	archive io.ReadCloser
	err     error
}

func (i *fakeBackups) Create(db.ConnInfo, metadata.Origin, string) (*metadata.Metadata, error) {
	if i.err != nil {
		return nil, errors.Trace(i.err)
	}
	return i.meta, nil
}

func (i *fakeBackups) Get(string) (*metadata.Metadata, io.ReadCloser, error) {
	if i.err != nil {
		return nil, nil, errors.Trace(i.err)
	}
	return i.meta, i.archive, nil
}
