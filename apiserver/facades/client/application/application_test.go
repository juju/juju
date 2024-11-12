// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/rpc/params"
)

type applicationSuite struct {
	baseSuite
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) TestAPIConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, nil, s.modelInfo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *applicationSuite) TestAPIServiceConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, nil, s.modelInfo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *applicationSuite) TestDeployBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.newAPI(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, gc.ErrorMatches, "deploy blocked")
}
