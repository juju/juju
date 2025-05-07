// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type agentSuite struct {
	testhelpers.IsolationSuite

	passwordService *MockAgentPasswordService
}

var _ = tc.Suite(&agentSuite{})

func (s *agentSuite) TestSetUnitPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(nil)

	api := &AgentAPI{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: nil,
			},
		},
	})
}

func (s *agentSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(applicationerrors.UnitNotFound)

	api := &AgentAPI{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: apiservererrors.ServerError(errors.NotFoundf(`unit "foo/1"`)),
			},
		},
	})
}

func (s *agentSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.passwordService = NewMockAgentPasswordService(ctrl)

	return ctrl
}

func alwaysAllow(context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return true
	}, nil
}
