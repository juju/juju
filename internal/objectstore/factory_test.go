// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type objectStoreFactorySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&objectStoreFactorySuite{})

func (s *objectStoreFactorySuite) TestNewObjectStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can create an object store with the default backend.

	obj, err := ObjectStoreFactory(
		context.Background(),
		DefaultBackendType(),
		"inferi",
		WithLogger(loggertesting.WrapCheckLog(c)),
		WithMetadataService(stubMetadataService{}),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obj, gc.NotNil)

	workertest.CleanKill(c, obj)
}

func (s *objectStoreFactorySuite) TestNewObjectStoreInvalidBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := ObjectStoreFactory(
		context.Background(),
		objectstore.BackendType("blah"),
		"inferi",
		WithLogger(loggertesting.WrapCheckLog(c)),
		WithMetadataService(stubMetadataService{}),
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *objectStoreFactorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	return ctrl
}

type stubMetadataService struct{}

func (stubMetadataService) ObjectStore() objectstore.ObjectStoreMetadata {
	return nil
}
