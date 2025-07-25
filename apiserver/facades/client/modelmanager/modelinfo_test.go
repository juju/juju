// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/objectstore"
)

type modelInfoSuite struct{}

func TestModelInfoSuite(t *testing.T) {
	tc.Run(t, &modelInfoSuite{})
}

func (s *modelInfoSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Test ModelInfo() with readAccess;
- Test ModelInfo() ErrorInvalidTag;
- Test ModelInfo() ErrorGetModelNotFound;
- Test ModelInfo() ErrorNoModelUsers;
- Test ModelInfo() ErrorNoAccess;
- Test ModelInfo() - running migration status;
`)
}

type mockObjectStore struct {
	objectstore.ObjectStore
}
