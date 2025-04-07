// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/unit"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type passwordSuite struct {
	testing.IsolationSuite

	agentPasswordService *mocks.MockAgentPasswordService
}

var _ = gc.Suite(&passwordSuite{})

func (s *passwordSuite) TestSetPasswordsForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(nil)

	changer := common.NewPasswordChanger(s.agentPasswordService, nil, alwaysAllow)
	results, err := changer.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      "unit-foo/1",
			Password: "password",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}},
	})
}

func (s *passwordSuite) TestSetPasswordsForUnitError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(internalerrors.Errorf("boom"))

	changer := common.NewPasswordChanger(s.agentPasswordService, nil, alwaysAllow)
	results, err := changer.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      "unit-foo/1",
			Password: "password",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(internalerrors.Errorf(`setting password for "unit-foo-1": boom`)),
		}},
	})
}

func (s *passwordSuite) TestSetPasswordsForUnitNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentPasswordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(agentpassworderrors.UnitNotFound)

	changer := common.NewPasswordChanger(s.agentPasswordService, nil, alwaysAllow)
	results, err := changer.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      "unit-foo/1",
			Password: "password",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.NotFoundf(`unit "foo/1"`)),
		}},
	})
}

func (s *passwordSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentPasswordService = mocks.NewMockAgentPasswordService(ctrl)

	return ctrl
}

func alwaysAllow() (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return true
	}, nil
}
