// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	envconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state/multiwatcher"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/imagemetadataworker"
	workertesting "github.com/juju/juju/worker/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	newCalled bool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.newCalled = false
	s.PatchValue(&imagemetadataworker.NewWorker,
		func(a *imagemetadata.Client) worker.Worker {
			s.newCalled = true
			return nil
		},
	)
}

func (s *ManifoldSuite) TestMachine(c *gc.C) {
	config := imagemetadataworker.ManifoldConfig(workertesting.PostUpgradeManifoldTestConfig())
	_, err := workertesting.RunPostUpgradeManifold(
		imagemetadataworker.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{machineJob: multiwatcher.JobManageModel})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestMachineNotModelManagerErrors(c *gc.C) {
	config := imagemetadataworker.ManifoldConfig(workertesting.PostUpgradeManifoldTestConfig())
	_, err := workertesting.RunPostUpgradeManifold(
		imagemetadataworker.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		&fakeAPIConn{machineJob: multiwatcher.JobHostUnits})
	c.Assert(err, gc.Equals, dependency.ErrMissing)
	c.Assert(s.newCalled, jc.IsFalse)
}

func (s *ManifoldSuite) TestNonMachineAgent(c *gc.C) {
	config := imagemetadataworker.ManifoldConfig(workertesting.PostUpgradeManifoldTestConfig())
	_, err := workertesting.RunPostUpgradeManifold(
		imagemetadataworker.Manifold(config),
		&fakeAgent{tag: names.NewUnitTag("foo/0")},
		&fakeAPIConn{})
	c.Assert(err, gc.ErrorMatches, "this manifold may only be used inside a machine agent")
	c.Assert(s.newCalled, jc.IsFalse)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}

type dummyProvider struct {
	environs.EnvironProvider
}

type dummyEnv struct {
	environs.Environ
	simplestreams.HasRegion
}

func (d *dummyProvider) Open(cfg *envconfig.Config) (environs.Environ, error) {
	return &dummyEnv{}, nil
}

type fakeAPIConn struct {
	api.Connection
	machineJob multiwatcher.MachineJob
}

func (f *fakeAPIConn) APICall(objType string, version int, id, request string, args interface{}, response interface{}) error {
	switch res := response.(type) {
	case *params.AgentGetEntitiesResults:

		res.Entities = []params.AgentGetEntitiesResult{
			{Jobs: []multiwatcher.MachineJob{
				f.machineJob,
			}},
		}
	case *params.ModelConfigResult:
		registered := &dummyProvider{}
		environs.RegisterProvider("someprovider", registered)
		res.Config = params.ModelConfig(coretesting.FakeConfig())
	}

	return nil
}

func (*fakeAPIConn) BestFacadeVersion(facade string) int {
	return 42
}

func (f *fakeAPIConn) Agent() *apiagent.State {
	return apiagent.NewState(f)
}
