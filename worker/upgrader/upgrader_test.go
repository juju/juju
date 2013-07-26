// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/upgrader"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type UpgraderSuite struct {
	jujutesting.JujuConnSuite

	machine *state.Machine
	state   *api.State
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
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	s.state = s.OpenAPIAsMachine(c, s.machine.Tag(), "test-password", "fake_nonce")
}

func (s *UpgraderSuite) TearDownTest(c *gc.C) {
	if s.state != nil {
		s.state.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

// primeTools sets up the current version of the tools to vers and
// makes sure that they're available JujuConnSuite's DataDir.
func (s *UpgraderSuite) primeTools(c *gc.C, vers version.Binary) *tools.Tools {
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, gc.IsNil)
	version.Current = vers
	agentTools := s.uploadTools(c, vers)
	resp, err := http.Get(agentTools.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	err = tools.UnpackTools(s.DataDir(), agentTools, resp.Body)
	c.Assert(err, gc.IsNil)
	return agentTools
}

// uploadTools uploads fake tools with the given version number
// to the dummy environment's storage and returns a tools
// value describing them.
func (s *UpgraderSuite) uploadTools(c *gc.C, vers version.Binary) *tools.Tools {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	storage := s.Conn.Environ.Storage()
	err := storage.Put(tools.StorageName(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, gc.IsNil)
	url, err := s.Conn.Environ.Storage().URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	return &tools.Tools{URL: url, Version: vers}
}

func (s *UpgraderSuite) TestUpgraderSetsTools(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-foo-bar")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, gc.IsNil)
	agentTools := s.primeTools(c, vers)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	u := upgrader.NewUpgrader(s.state.Upgrader(), s.DataDir(), s.machine.Tag())
	c.Assert(u.Stop(), gc.IsNil)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(gotTools, gc.DeepEquals, agentTools)
}

func (s *UpgraderSuite) TestUpgraderSetToolsEvenWithNoToolsToRead(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-foo-bar")
	err := statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, gc.IsNil)
	s.primeTools(c, vers)
	err = os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, gc.IsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	u := upgrader.NewUpgrader(s.state.Upgrader(), s.DataDir(), s.machine.Tag())
	c.Assert(u.Stop(), gc.IsNil)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(gotTools, gc.DeepEquals, &tools.Tools{Version: version.Current})
}
