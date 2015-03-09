// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&provisionerSuite{})

type provisionerSuite struct {
	// TODO(wallyworld) remove JujuConnSuite
	jujutesting.JujuConnSuite

	factory    *factory.Factory
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *storageprovisioner.StorageProvisionerAPI
}

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.factory = factory.NewFactory(s.State)
	s.resources = common.NewResources()
	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	var err error
	s.api, err = storageprovisioner.NewStorageProvisionerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *provisionerSuite) TestNewStorageProvisionerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := storageprovisioner.NewStorageProvisionerAPI(s.State, common.NewResources(), s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *provisionerSuite) setupVolumes(c *gc.C) {
	s.factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("inst-id"),
		Nonce:      "nonce",
		Volumes: []state.MachineVolumeParams{
			{Volume: state.VolumeParams{Pool: "loop", Size: 1024}},
			{Volume: state.VolumeParams{Pool: "loop", Size: 2048}},
		},
	})
	// Only provision the first volume.
	err := s.State.SetVolumeInfo(names.NewVolumeTag("0"), state.VolumeInfo{
		Serial:   "123",
		VolumeId: "abc",
		Size:     1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Make another machine for tests to use.
	s.factory.MakeMachine(c, nil)
}

func (s *provisionerSuite) TestVolumes(c *gc.C) {
	s.setupVolumes(c)
	results, err := s.api.Volumes(params.Entities{
		Entities: []params.Entity{{"volume-0"}, {"volume-1"}, {"volume-42"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{Result: params.Volume{VolumeTag: "volume-0", VolumeId: "abc", Serial: "123", Size: 1024}},
			{Error: common.ServerError(errors.NotProvisionedf(`volume "1"`))},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumesEmptyArgs(c *gc.C) {
	results, err := s.api.Volumes(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *provisionerSuite) TestVolumeParams(c *gc.C) {
	s.setupVolumes(c)
	results, err := s.api.VolumeParams(params.Entities{
		Entities: []params.Entity{{"volume-0"}, {"volume-1"}, {"volume-42"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeParamsResults{
		Results: []params.VolumeParamsResult{
			{Error: &params.Error{`volume "0" is already provisioned`, ""}},
			{Result: params.VolumeParams{VolumeTag: "volume-1", Size: 2048, Provider: "loop", MachineTag: "machine-0"}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumeParamsEmptyArgs(c *gc.C) {
	results, err := s.api.VolumeParams(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *provisionerSuite) TestWatchVolumes(c *gc.C) {
	s.setupVolumes(c)
	s.factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{{"machine-0"}, {"machine-1"}, {"machine-42"}}}
	result, err := s.api.WatchVolumes(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(result.Results[0].Changes)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{"0", "1"}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	v0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, v0Watcher)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{"volume-0"}, {"volume-1"}, {"volume-42"}}}
	result, err := s.api.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Alive},
			{Error: common.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *provisionerSuite) TestEnsureDead(c *gc.C) {
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{"volume-0"}, {"volume-1"}, {"volume-42"}}}
	result, err := s.api.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) - this test will be updated when EnsureDead is supported
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: common.ServerError(common.NotSupportedError(names.NewVolumeTag("0"), "ensuring death"))},
			{Error: common.ServerError(common.NotSupportedError(names.NewVolumeTag("1"), "ensuring death"))},
			{Error: common.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}
