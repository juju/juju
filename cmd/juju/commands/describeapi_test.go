// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package commands

import (
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
)

type DescribeAPISuite struct {
	testing.IsolationSuite

	apiServer *MockAPIServer
	registry  *MockRegistry
}

var _ = gc.Suite(&DescribeAPISuite{})

func (s *DescribeAPISuite) TestResult(c *gc.C) {
	defer s.setup(c).Finish()

	cmd := s.scenario(c,
		s.expectList,
		s.expectGetType,
		s.expectClose,
	)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "[{\"Name\":\"Resources\",\"Version\":4,\"Schema\":{\"type\":\"object\"}}]\n")
}

func (s *DescribeAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiServer = NewMockAPIServer(ctrl)
	s.registry = NewMockRegistry(ctrl)

	return ctrl
}

func (s *DescribeAPISuite) scenario(c *gc.C, behaviours ...func()) cmd.Command {
	for _, b := range behaviours {
		b()
	}

	return &describeAPICommand{
		apiServer: s.apiServer,
	}
}

func (s *DescribeAPISuite) expectList() {
	aExp := s.apiServer.EXPECT()
	aExp.AllFacades().Return(s.registry)

	rExp := s.registry.EXPECT()
	rExp.List().Return([]facade.Description{
		{
			Name:     "Resources",
			Versions: []int{1, 2, 3, 4},
		},
	})
}

type ResourcesFacade struct{}

func (ResourcesFacade) Resources(params []string) ([]string, error) {
	return nil, nil
}

func (s *DescribeAPISuite) expectGetType() {
	rExp := s.registry.EXPECT()
	rExp.GetType("Resources", 4).Return(reflect.TypeOf(ResourcesFacade{}), nil)
}

func (s *DescribeAPISuite) expectClose() {
	aExp := s.apiServer.EXPECT()
	aExp.Close().Return(nil)
}
