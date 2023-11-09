// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
}

func (s *ObjectStoreFactorySuite) TestNewObjectStoreInvalidBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// As file backed object stores are not supported, ensure we get an error
	// when trying to create one.

	_, err := ObjectStoreFactory(context.Background(), FileBackend, "inferi", WithMongoSession(s.session), WithLogger(jujutesting.NewCheckLogger(c)))
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *ObjectStoreFactorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.session = NewMockMongoSession(ctrl)
	return ctrl
}
