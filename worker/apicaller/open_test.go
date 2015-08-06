// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"fmt"

	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
)

type apiOpenSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&apiOpenSuite{})

func (s *apiOpenSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(apicaller.CheckProvisionedStrategy, utils.AttemptStrategy{})
}

func (s *apiOpenSuite) TestOpenAPIStateReplaceErrors(c *gc.C) {
	type replaceErrors struct {
		openErr    error
		replaceErr error
	}
	var apiError error
	s.PatchValue(apicaller.OpenAPIForAgent, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		return nil, apiError
	})
	errReplacePairs := []replaceErrors{{
		fmt.Errorf("blah"), nil,
	}, {
		openErr:    &params.Error{Code: params.CodeNotProvisioned},
		replaceErr: worker.ErrTerminateAgent,
	}, {
		openErr:    &params.Error{Code: params.CodeUnauthorized},
		replaceErr: worker.ErrTerminateAgent,
	}}
	for i, test := range errReplacePairs {
		c.Logf("test %d", i)
		apiError = test.openErr
		_, _, err := apicaller.OpenAPIState(fakeAPIOpenConfig{}, nil)
		if test.replaceErr == nil {
			c.Check(err, gc.Equals, test.openErr)
		} else {
			c.Check(err, gc.Equals, test.replaceErr)
		}
	}
}

func (s *apiOpenSuite) TestOpenAPIStateWaitsProvisioned(c *gc.C) {
	s.PatchValue(&apicaller.CheckProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(apicaller.OpenAPIForAgent, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		called++
		if called == apicaller.CheckProvisionedStrategy.Min-1 {
			return nil, &params.Error{Code: params.CodeUnauthorized}
		}
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := apicaller.OpenAPIState(fakeAPIOpenConfig{}, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	c.Assert(called, gc.Equals, apicaller.CheckProvisionedStrategy.Min-1)
}

func (s *apiOpenSuite) TestOpenAPIStateWaitsProvisionedGivesUp(c *gc.C) {
	s.PatchValue(&apicaller.CheckProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(apicaller.OpenAPIForAgent, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		called++
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := apicaller.OpenAPIState(fakeAPIOpenConfig{}, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	// +1 because we always attempt at least once outside the attempt strategy
	// (twice if the API server initially returns CodeUnauthorized.)
	c.Assert(called, gc.Equals, apicaller.CheckProvisionedStrategy.Min+1)
}

func (s *apiOpenSuite) TestOpenAPIStateRewritesInitialPassword(c *gc.C) {
	c.Fatalf("not done")
}

type fakeAPIOpenConfig struct {
	agent.Config
}

func (fakeAPIOpenConfig) APIInfo() *api.Info              { return &api.Info{} }
func (fakeAPIOpenConfig) OldPassword() string             { return "old" }
func (fakeAPIOpenConfig) Jobs() []multiwatcher.MachineJob { return []multiwatcher.MachineJob{} }
