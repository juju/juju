// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

type applicationSuite struct {
	baseSuite

	application *MockApplication
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) TestSetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.application = NewMockApplication(ctrl)

	return ctrl
}

func (s *applicationSuite) setupAPI(c *gc.C) {
	s.expectAuthClient(c)
	s.expectAnyPermissions(c)
	s.expectAnyChangeOrRemoval(c)

	s.newIAASAPI(c)
}

func (s *applicationSuite) expectApplication(c *gc.C, name string) {
	s.backend.EXPECT().Application(name).Return(s.application, nil)
}

func (s *applicationSuite) expectCharm(c *gc.C, name string) {
	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), applicationcharm.GetCharmArgs{
		Name:     name,
		Revision: ptr(42),
	}).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(internalcharm.Charm{}, nil)
}
