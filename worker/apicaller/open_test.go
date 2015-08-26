// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

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
)

type OpenAPIStateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&OpenAPIStateSuite{})

func (s *OpenAPIStateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&checkProvisionedStrategy, utils.AttemptStrategy{})
}

func (s *OpenAPIStateSuite) TestOpenAPIStateReplaceErrors(c *gc.C) {
	type replaceErrors struct {
		openErr    error
		replaceErr error
	}
	var apiError error
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
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
		_, _, err := OpenAPIState(fakeAgent{})
		if test.replaceErr == nil {
			c.Check(err, gc.Equals, test.openErr)
		} else {
			c.Check(err, gc.Equals, test.replaceErr)
		}
	}
}

func (s *OpenAPIStateSuite) TestOpenAPIStateWaitsProvisioned(c *gc.C) {
	s.PatchValue(&checkProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		called++
		if called == checkProvisionedStrategy.Min-1 {
			return nil, &params.Error{Code: params.CodeUnauthorized}
		}
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := OpenAPIState(fakeAgent{})
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	c.Assert(called, gc.Equals, checkProvisionedStrategy.Min-1)
}

func (s *OpenAPIStateSuite) TestOpenAPIStateWaitsProvisionedGivesUp(c *gc.C) {
	s.PatchValue(&checkProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		called++
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := OpenAPIState(fakeAgent{})
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	// +1 because we always attempt at least once outside the attempt strategy
	// (twice if the API server initially returns CodeUnauthorized.)
	c.Assert(called, gc.Equals, checkProvisionedStrategy.Min+1)
}

type fakeAgent struct {
	agent.Agent
}

func (fakeAgent) CurrentConfig() agent.Config {
	return fakeAPIOpenConfig{}
}

type fakeAPIOpenConfig struct {
	agent.Config
}

func (fakeAPIOpenConfig) APIInfo() *api.Info              { return &api.Info{} }
func (fakeAPIOpenConfig) OldPassword() string             { return "old" }
func (fakeAPIOpenConfig) Jobs() []multiwatcher.MachineJob { return []multiwatcher.MachineJob{} }
