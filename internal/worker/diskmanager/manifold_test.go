// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apidiskmanager "github.com/juju/juju/api/agent/diskmanager"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/diskmanager"
	coretesting "github.com/juju/juju/testing"
)

type manifoldSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestMachineDiskmanager(c *gc.C) {

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, response interface{},
		) error {

			// We don't test the api call. We test that NewWorker is
			// passed the expected arguments.
			return nil
		})

	s.PatchValue(&diskmanager.NewWorker, func(l diskmanager.ListBlockDevicesFunc, b diskmanager.BlockDeviceSetter) worker.Worker {
		called = true

		c.Assert(l, gc.FitsTypeOf, diskmanager.DefaultListBlockDevices)
		c.Assert(b, gc.NotNil)

		api, ok := b.(*apidiskmanager.State)
		c.Assert(ok, jc.IsTrue)
		c.Assert(api, gc.NotNil)

		return nil
	})

	a := &dummyAgent{
		tag: names.NewMachineTag("1"),
		jobs: []model.MachineJob{
			model.JobManageModel,
		},
	}

	_, err := diskmanager.NewWorkerFunc(a, apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type dummyAgent struct {
	agent.Agent
	tag  names.Tag
	jobs []model.MachineJob
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
	jobs []model.MachineJob
}

func (c dummyCfg) Tag() names.Tag {
	return c.tag
}
