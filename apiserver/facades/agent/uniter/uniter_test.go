// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

type uniterSuite struct {
	testing.IsolationSuite

	applicationService *MockApplicationService

	uniter *UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) TestCharmArchiveSha256Local(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), domaincharm.GetCharmArgs{
		Source:   domaincharm.LocalSource,
		Name:     "foo",
		Revision: ptr(1),
	}).Return(id, nil)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "local:foo-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Charmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), domaincharm.GetCharmArgs{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: ptr(1),
	}).Return(id, nil)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Errors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), domaincharm.GetCharmArgs{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: ptr(1),
	}).Return(id, applicationerrors.CharmNotFound)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), domaincharm.GetCharmArgs{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: ptr(2),
	}).Return(id, nil)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("", applicationerrors.CharmNotFound)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), domaincharm.GetCharmArgs{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: ptr(3),
	}).Return(id, nil)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), id).Return("", applicationerrors.CharmNotResolved)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
			{URL: "ch:foo-2"},
			{URL: "ch:foo-3"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: &params.Error{Message: `charm "foo-1" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-2" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-3" not available`, Code: params.CodeNotYetAvailable}},
		},
	})
}

func (s *uniterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)

	s.uniter = &UniterAPI{
		applicationService: s.applicationService,
	}

	return ctrl
}
