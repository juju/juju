// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/errors"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
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

	machine       *state.Machine
	state         *api.State
	oldRetryAfter func() <-chan time.Time
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
	s.oldRetryAfter = *upgrader.RetryAfter
}

func (s *UpgraderSuite) TearDownTest(c *gc.C) {
	*upgrader.RetryAfter = s.oldRetryAfter
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
	// TODO(rog) make UploadFakeToolsVersion in environs/testing
	// sufficient for this use case.
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

	u := upgrader.New(s.state.Upgrader(), s.machine.Tag(), s.DataDir())
	statetesting.AssertStop(c, u)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(gotTools, gc.DeepEquals, agentTools)
}

func (s *UpgraderSuite) TestUpgraderSetToolsEvenWithNoToolsToRead(c *gc.C) {
	vers := version.MustParseBinary("5.4.3-foo-bar")
	s.primeTools(c, vers)
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, gc.IsNil)

	_, err = s.machine.AgentTools()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	err = statetesting.SetAgentVersion(s.State, vers.Number)
	c.Assert(err, gc.IsNil)

	u := upgrader.New(s.state.Upgrader(), s.machine.Tag(), s.DataDir())
	statetesting.AssertStop(c, u)
	s.machine.Refresh()
	gotTools, err := s.machine.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(gotTools, gc.DeepEquals, &tools.Tools{Version: version.Current})
}

func (s *UpgraderSuite) TestUpgraderUpgradesImmediately(c *gc.C) {
	oldTools := s.primeTools(c, version.MustParseBinary("5.4.3-foo-bar"))
	newTools := s.uploadTools(c, version.MustParseBinary("5.4.5-foo-bar"))

	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, gc.IsNil)

	// Make the download take a while so that we verify that
	// the download happens before the upgrader checks if
	// it's been stopped.
	dummy.SetStorageDelay(coretesting.ShortWait)

	u := upgrader.New(s.state.Upgrader(), s.machine.Tag(), s.DataDir())
	err = u.Stop()
	c.Assert(err, gc.DeepEquals, &upgrader.UpgradeReadyError{
		AgentName: s.machine.Tag(),
		OldTools:  oldTools,
		NewTools:  newTools,
		DataDir:   s.DataDir(),
	})
	foundTools, err := tools.ReadTools(s.DataDir(), newTools.Version)
	c.Assert(err, gc.IsNil)
	c.Assert(foundTools, gc.DeepEquals, newTools)
}

func (s *UpgraderSuite) TestUpgraderRetryAndChanged(c *gc.C) {
	oldTools := s.primeTools(c, version.MustParseBinary("5.4.3-foo-bar"))
	newTools := s.uploadTools(c, version.MustParseBinary("5.4.5-foo-bar"))

	err := statetesting.SetAgentVersion(s.State, newTools.Version.Number)
	c.Assert(err, gc.IsNil)

	retryc := make(chan time.Time)
	*upgrader.RetryAfter = func() <-chan time.Time {
		c.Logf("replacement retry after")
		return retryc
	}
	dummy.Poison(s.Conn.Environ.Storage(), tools.StorageName(newTools.Version), fmt.Errorf("a non-fatal dose"))
	u := upgrader.New(s.state.Upgrader(), s.machine.Tag(), s.DataDir())
	defer u.Stop()

	for i := 0; i < 3; i++ {
		select {
		case retryc <- time.Now():
		case <-time.After(coretesting.LongWait):
			c.Fatalf("upgrader did not retry (attempt %d)", i)
		}
	}

	// Make it upgrade to some newer tools that can be
	// downloaded ok; it should stop retrying, download
	// the newer tools and exit.
	newerTools := s.uploadTools(c, version.MustParseBinary("5.4.6-foo-bar"))
	err = statetesting.SetAgentVersion(s.State, newerTools.Version.Number)
	c.Assert(err, gc.IsNil)

	s.BackingState.StartSync()
	done := make(chan error)
	go func() {
		done <- u.Wait()
	}()
	select {
	case err := <-done:
		c.Assert(err, gc.DeepEquals, &upgrader.UpgradeReadyError{
			AgentName: s.machine.Tag(),
			OldTools:  oldTools,
			NewTools:  newerTools,
			DataDir:   s.DataDir(),
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("upgrader did not quit after upgrading")
	}
}

func (s *UpgraderSuite) TestChangeAgentTools(c *gc.C) {
	oldTools := &tools.Tools{
		Version: version.MustParseBinary("1.2.3-arble-bletch"),
	}
	newTools := s.primeTools(c, version.MustParseBinary("5.4.3-foo-bar"))
	ugErr := &upgrader.UpgradeReadyError{
		AgentName: "anAgent",
		OldTools:  oldTools,
		NewTools:  newTools,
		DataDir:   s.DataDir(),
	}
	err := ugErr.ChangeAgentTools()
	c.Assert(err, gc.IsNil)
	link, err := os.Readlink(tools.ToolsDir(s.DataDir(), "anAgent"))
	c.Assert(err, gc.IsNil)
	c.Assert(link, gc.Equals, newTools.Version.String())
}
