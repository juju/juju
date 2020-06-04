// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/collections/set"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/network"
	jujunetwork "github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
)

type lxdBridgeSelectionSuite struct {
	coretesting.BaseSuite
	agentCfg agent.ConfigSetterWriter

	ovsBridges set.Strings
}

var _ = gc.Suite(&lxdBridgeSelectionSuite{})

func (s *lxdBridgeSelectionSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxd tests on windows")
	}

	var err error
	s.agentCfg, err = agent.NewAgentConfig(agent.AgentConfigParams{
		Paths:             agent.NewPathsWithDefaults(agent.Paths{DataDir: "/not/used/here"}),
		Tag:               names.NewMachineTag("1"),
		UpgradedToVersion: jujuversion.Current,
		Password:          "dummy-secret",
		Nonce:             "nonce",
		APIAddresses:      []string{"10.0.0.1:1234"},
		CACert:            coretesting.CACert,
		Controller:        coretesting.ControllerTag,
		Model:             coretesting.ModelTag,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.ovsBridges = set.NewStrings()
	lookupOvsManagedBridges = func() (set.Strings, error) { return s.ovsBridges, nil }
}

func (s *lxdBridgeSelectionSuite) TearDownTest(c *gc.C) {
	lookupOvsManagedBridges = network.OvsManagedBridges
}

func (s *lxdBridgeSelectionSuite) TestPreferredBridgeConfigPriority(c *gc.C) {
	s.agentCfg.SetValue(agent.LxdBridge, "the-lxD-bridge")
	s.agentCfg.SetValue(agent.LxcBridge, "the-lxC-bridge")

	// With both a preferred lxd and lxc bridge, we should pick the lxd one
	got, err := selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, "the-lxD-bridge", gc.Commentf("expected LxdBridge setting value to be picked"))

	// With only a preferred lxc bridge we pick that by default.
	s.agentCfg.SetValue(agent.LxdBridge, "")
	got, err = selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, "the-lxC-bridge", gc.Commentf("expected LxcBridge setting value to be picked"))
}

func (s *lxdBridgeSelectionSuite) TestSelectBridgeOVSBridges(c *gc.C) {
	s.agentCfg.SetValue(agent.LxdBridge, "the-lxD-bridge")
	s.ovsBridges.Add("ovsbr0")

	// With both a preferred lxd and an OVS-managed bridge, the lxd one has priority.
	got, err := selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, "the-lxD-bridge", gc.Commentf("expected LxdBridge setting value to be picked"))

	// A single OVS-managed bridge should be picked by default if no
	// preferred bridge options were passed to the agent by the
	// operator.
	s.agentCfg.SetValue(agent.LxdBridge, "")
	got, err = selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, "ovsbr0", gc.Commentf("expected the single OVS-managed bridge to be picked"))

	// With no OVS bridges present and no preferred bridge options passed
	// in by the operator we should pick a sane default.
	s.ovsBridges.Remove("ovsbr0")
	got, err = selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, jujunetwork.DefaultLXDBridge, gc.Commentf("expected the default LXD bridge to be picked as a fallback"))
}

func (s *lxdBridgeSelectionSuite) TestSelectBridgeWithMultipleOVSBridges(c *gc.C) {
	s.ovsBridges.Add("ovsbr0")
	s.ovsBridges.Add("ovsbr1")

	// With multiple OVS-bridges present and no operator preference we
	// should fall back to the default LXD bridge
	got, err := selectBridgeDevice(s.agentCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, jujunetwork.DefaultLXDBridge, gc.Commentf("expected the default LXD bridge to be picked as a fallback"))
}
