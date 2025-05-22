// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/mocks"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/testhelpers"
)

type apiCallerSuite struct {
	testhelpers.IsolationSuite

	apiCaller *mocks.MockAPICaller
}

func TestApiCallerSuite(t *stdtesting.T) {
	tc.Run(t, &apiCallerSuite{})
}

func (s *apiCallerSuite) TestNewFacadeCaller(c *tc.C) {
	defer s.setupMocks(c).Finish()

	facade := base.NewFacadeCaller(s.apiCaller, "Foo")
	c.Assert(facade, tc.NotNil)
	c.Check(facade.(Tracer).Tracer(), tc.IsNil)
}

func (s *apiCallerSuite) TestNewFacadeCallerWithTracer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	facade := base.NewFacadeCaller(s.apiCaller, "Foo", base.WithTracer(coretrace.NoopTracer{}))
	c.Assert(facade, tc.NotNil)
	c.Check(facade.(Tracer).Tracer(), tc.Equals, coretrace.NoopTracer{})
}

func (s *apiCallerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.apiCaller.EXPECT().BestFacadeVersion("Foo").Return(1)

	return ctrl
}

type Tracer interface {
	Tracer() coretrace.Tracer
}
