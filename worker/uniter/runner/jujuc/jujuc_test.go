// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc/mocks"
)

type jujucSuite struct {
	mockContext *mocks.MockContext
}

func (s *jujucSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockContext = mocks.NewMockContext(ctrl)
	return ctrl
}
