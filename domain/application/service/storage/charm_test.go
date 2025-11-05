// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// charmStorageValidationSuite is a test suite for validating charm storage
// definitions.
type charmStorageValidationSuite struct {
	state               *MockState
	storagePoolProvider *MockStoragePoolProvider
}

// TestCharmStorageValidationSuite runs the tests contained with
// [charmStorageValidationSuite].
func TestCharmStorageValidationSuite(t *testing.T) {
	tc.Run(t, &charmStorageValidationSuite{})
}

func (s *charmStorageValidationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.storagePoolProvider = NewMockStoragePoolProvider(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.storagePoolProvider = nil
	})
	return ctrl
}

// TestProhibitedLocation tests that a prohibited storage location is reported
// back to the callers as
// [domainapplicationerrors.CharmStorageLocationProhibited] error.
func (s *charmStorageValidationSuite) TestProhibitedLocation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storage := map[string]internalcharm.Storage{
		"correct": {
			Location: "/var/lib/postgresql/data",
		},
		"bad": {
			Location: "/var/lib/juju/storage/data",
		},
	}

	svc := NewService(s.state, s.storagePoolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateCharmStorage(c.Context(), storage)

	prohibitedErr, has :=
		errors.AsType[domainapplicationerrors.CharmStorageLocationProhibited](err)
	c.Assert(has, tc.IsTrue)
	c.Check(prohibitedErr, tc.Equals, domainapplicationerrors.CharmStorageLocationProhibited{
		CharmStorageLocation: "/var/lib/juju/storage/data",
		CharmStorageName:     "bad",
		ProhibitedLocation:   "/var/lib/juju/storage",
	})
}

// TestNonProhibitedLocation tests the happy path of validating charm storage.
func (s *charmStorageValidationSuite) TestNonProhibitedLocation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storage := map[string]internalcharm.Storage{
		"correct1": {
			Location: "/var/lib/postgresql/data",
		},
		"correct2": {
			Location: "/mnt/data/storage",
		},
	}

	svc := NewService(s.state, s.storagePoolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateCharmStorage(c.Context(), storage)
	c.Check(err, tc.IsNil)
}
