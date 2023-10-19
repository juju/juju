// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/mocks"
	coretrace "github.com/juju/juju/core/trace"
)

type apiCallerSuite struct {
	testing.IsolationSuite

	apiCaller *mocks.MockAPICaller
}

var _ = gc.Suite(&apiCallerSuite{})

func (s *apiCallerSuite) TestNewFacadeCaller(c *gc.C) {
	defer s.setupMocks(c).Finish()

	facade := base.NewFacadeCaller(s.apiCaller, "Foo")
	c.Assert(facade, gc.NotNil)
	c.Check(facade.(Tracer).Tracer(), gc.Equals, coretrace.NoopTracer{})
}

func (s *apiCallerSuite) TestNewFacadeCallerWithTracer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	facade := base.NewFacadeCaller(s.apiCaller, "Foo", base.WithTracer(coretrace.NoopTracer{}))
	c.Assert(facade, gc.NotNil)
	c.Check(facade.(Tracer).Tracer(), gc.Equals, coretrace.NoopTracer{})
}

func (s *apiCallerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.apiCaller.EXPECT().BestFacadeVersion("Foo").Return(1)

	return ctrl
}

type Tracer interface {
	Tracer() coretrace.Tracer
}
