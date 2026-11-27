// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	coretesting "github.com/juju/juju/internal/testing"
)

type nameSuite struct {
	coretesting.BaseSuite
}

func TestNameSuite(t *testing.T) {
	tc.Run(t, &nameSuite{})
}

func (s *nameSuite) TestParseName(c *tc.C) {
	for _, valid := range []string{"data", "shared-fs", "pg-data1"} {
		got, err := storage.ParseName(valid)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got.String(), tc.Equals, valid)
	}

	for _, invalid := range []string{
		"",
		"0",
		"pg-data-1",
		"shared-fs/0",
		"-shared-fs",
		"shared-fs-",
		"shared--fs",
		"BAD_name",
		"shared fs",
	} {
		_, err := storage.ParseName(invalid)
		c.Check(err, tc.ErrorIs, storageerrors.InvalidStorageName, tc.Commentf("%q", invalid))
	}
}
