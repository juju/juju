// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"bytes"
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/upgrader"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ worker.NotifyWorker = (*upgrader.UpgradeWorker)(nil)

type UpgraderSuite struct {
	jujutesting.JujuConnSuite
	//SimpleToolsFixture

	machine *state.Machine
	apiState   *api.State
}

var _ = gc.Suite(&UpgraderSuite{})

func (s *UpgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword("test-password")
	c.Assert(err, gc.IsNil)

	//s.SimpleToolsFixture.SetUp(c, s.DataDir())
	s.apiState = s.OpenAPIAs(c, s.machine.Tag(), "test-password")
}

func (s *UpgraderSuite) TearDownTest(c *gc.C) {
	if s.apiState != nil {
		s.apiState.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	_, err := s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	upg := upgrader.NewUpgrader(s.apiState, s.machine.Tag(), upgrader.NilToolsManager{})
	c.Assert(upg.Stop(), gc.IsNil)
	s.machine.Refresh()
	ver, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(ver.Binary, gc.Equals, version.Current)
}

type UpgradeHandlerSuite struct {
	jujutesting.JujuConnSuite
	machine *state.Machine
	apiState   *api.State
}

func (s *UpgradeHandlerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword("test-password")
	c.Assert(err, gc.IsNil)

	//s.SimpleToolsFixture.SetUp(c, s.DataDir())
	s.apiState = s.OpenAPIAs(c, s.machine.Tag(), "test-password")
}

func (s *UpgradeHandlerSuite) TestUpgraderDoesNothingIfAgentVersionMatches(c *gc.C) {
	handler := upgrader.NewUpgradeHandler(s.apiState, s.machine.Tag())
	handler.Handle()
	c.Assert(handler.DownloadChannel(), gc.IsNil)
}

func (s *UpgradeHandlerSuite) TestUpgraderGrabsNewTools(c *gc.C) {
	handler := upgrader.NewUpgradeHandler(s.apiState, s.machine.Tag())
	handler.Handle()
}

func uploadTools(storage environs.Storage, c *gc.C, vers version.Binary) *state.Tools {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	err := storage.Put(tools.StorageName(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, gc.IsNil)
	url, err := storage.URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	return &state.Tools{URL: url, Binary: vers}
}
