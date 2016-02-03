// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/environment"
	apinetworker "github.com/juju/juju/api/networker"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/networker"
)

type manifoldSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestMachineNetworker(c *gc.C) {

	caller := basetesting.FacadeCallerFunc(
		func(request string, args, response interface{}) error {

			if result, ok := response.(*params.EnvironConfigResult); ok {
				result.Config = params.EnvironConfig(coretesting.FakeConfig())
			}
			return nil
		})

	apiCaller := api.Connection(&dummyConn{facadeCaller: caller})

	called := false
	s.PatchValue(&networker.NewNetworker, func(
		st apinetworker.State,
		agentConfig agent.Config,
		intrusiveMode bool,
		configBaseDir string,
	) (worker.Worker, error) {
		called = true

		c.Assert(st, gc.NotNil)
		c.Assert(intrusiveMode, jc.IsTrue)

		return nil, nil
	})

	a := &dummyAgent{
		tag: names.NewMachineTag("1"),
		jobs: []multiwatcher.MachineJob{
			multiwatcher.JobManageNetworking,
		},
	}

	_, err := networker.NewWorker(a, apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type dummyConn struct {
	api.Connection
	facadeCaller base.FacadeCaller
}

func (d *dummyConn) Environment() *environment.Facade {

	return &environment.Facade{
		EnvironWatcher:      common.NewEnvironWatcher(d.facadeCaller),
		ToolsVersionUpdater: environment.NewToolsVersionUpdater(d.facadeCaller),
	}
}

func (d *dummyConn) BestFacadeVersion(_ string) int {
	return 0
}

func (d *dummyConn) Agent() *apiagent.State {

	caller := basetesting.APICallerFunc(
		func(
			objType string,
			version int,
			id,
			request string,
			args,
			response interface{}) error {

			if res, ok := response.(*params.AgentGetEntitiesResults); ok {
				res.Entities = []params.AgentGetEntitiesResult{
					{
						Life:          "alive",
						Jobs:          []multiwatcher.MachineJob{multiwatcher.JobManageNetworking},
						ContainerType: instance.LXC,
					},
				}
			}
			return nil
		})

	return apiagent.NewState(caller)
}

type dummyAgent struct {
	agent.Agent
	tag  names.Tag
	jobs []multiwatcher.MachineJob
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return dummyCfg{
		tag:  a.tag,
		jobs: a.jobs,
	}
}

type dummyCfg struct {
	agent.Config
	tag  names.Tag
	jobs []multiwatcher.MachineJob
}

func (c dummyCfg) Tag() names.Tag {
	return c.tag
}
