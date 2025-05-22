// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type objectStoreFactorySuite struct {
	testhelpers.IsolationSuite
}

func TestObjectStoreFactorySuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &objectStoreFactorySuite{})
}

func (s *objectStoreFactorySuite) TestNewObjectStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can create an object store with the default backend.

	obj, err := ObjectStoreFactory(
		c.Context(),
		DefaultBackendType(),
		"inferi",
		WithLogger(loggertesting.WrapCheckLog(c)),
		WithMetadataService(stubMetadataService{}),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obj, tc.NotNil)

	workertest.CleanKill(c, obj)
}

func (s *objectStoreFactorySuite) TestNewObjectStoreInvalidBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := ObjectStoreFactory(
		c.Context(),
		objectstore.BackendType("blah"),
		"inferi",
		WithLogger(loggertesting.WrapCheckLog(c)),
		WithMetadataService(stubMetadataService{}),
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *objectStoreFactorySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	return ctrl
}

type stubMetadataService struct{}

func (stubMetadataService) ObjectStore() objectstore.ObjectStoreMetadata {
	return nil
}
