// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	jujutesting "github.com/juju/juju/testing"
)

type ObjectStoreFactorySuite struct {
	testing.IsolationSuite

	session *MockMongoSession
}

var _ = gc.Suite(&ObjectStoreFactorySuite{})

func (s *ObjectStoreFactorySuite) TestNewObjectStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can create an object store with the default backend.

	obj, err := ObjectStoreFactory(context.Background(), DefaultBackendType(), "inferi", WithMongoSession(s.session), WithLogger(jujutesting.NewCheckLogger(c)))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obj, gc.NotNil)

	workertest.CleanKill(c, obj)
}

func (s *ObjectStoreFactorySuite) TestNewObjectStoreInvalidBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := ObjectStoreFactory(context.Background(), objectstore.BackendType("blah"), "inferi", WithMongoSession(s.session), WithLogger(jujutesting.NewCheckLogger(c)))
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *ObjectStoreFactorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.session = NewMockMongoSession(ctrl)
	return ctrl
}
