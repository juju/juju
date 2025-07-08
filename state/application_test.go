// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/arch"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

type ApplicationSuite struct {
	ConnSuite

	charm *state.Charm
	mysql *state.Application
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.charm = s.AddTestingCharm(c, "mysql")
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ApplicationSuite) assertNeedsCleanup(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
}

func (s *ApplicationSuite) assertNoCleanup(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}

func (s *ApplicationSuite) TestSetCharm(c *gc.C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)
	url, force := s.mysql.CharmURL()
	c.Assert(*url, gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)

	// Add a compatible charm and force it.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err = s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	url, force = s.mysql.CharmURL()
	c.Assert(*url, gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
}

func (s *ApplicationSuite) TestSetCharmCharmOrigin(c *gc.C) {
	// Add a compatible charm.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)
	rev := sch.Revision()
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: &rev,
		Channel:  &state.Channel{Risk: "stable"},
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		},
	}
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: origin,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	obtainedOrigin := s.mysql.CharmOrigin()
	c.Assert(obtainedOrigin, gc.DeepEquals, origin)
}

func (s *ApplicationSuite) TestSetCharmUpdateChannelURLNoChange(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	origin := defaultCharmOrigin(sch.URL())
	// This is a workaround, AddCharm creates a local
	// charm, which cannot have a channel in the CharmOrigin.
	// However, we need to test changing the channel only.
	origin.Source = "charm-hub"
	origin.Channel = &state.Channel{
		Risk: "stable",
	}
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: origin,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	mOrigin := s.mysql.CharmOrigin()
	c.Assert(mOrigin.Channel, gc.NotNil)
	c.Assert(mOrigin.Channel.Risk, gc.DeepEquals, "stable")

	cfg.CharmOrigin.Channel.Risk = "candidate"
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().Channel.Risk, gc.DeepEquals, "candidate")
}

func (s *ApplicationSuite) TestLXDProfileSetCharm(c *gc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile")
	app := s.AddTestingApplication(c, "lxd-profile", charm)

	c.Assert(charm.LXDProfile(), gc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.LXDProfile(), gc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err = app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	url, force = app.CharmURL()
	c.Assert(*url, gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	c.Assert(charm.LXDProfile(), gc.DeepEquals, ch.LXDProfile())
}

func (s *ApplicationSuite) TestLXDProfileFailSetCharm(c *gc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile-fail")
	app := s.AddTestingApplication(c, "lxd-profile-fail", charm)

	c.Assert(charm.LXDProfile(), gc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.LXDProfile(), gc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile-fail", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, ".*validating lxd profile: invalid lxd-profile\\.yaml.*")
}

func (s *ApplicationSuite) TestLXDProfileFailWithForceSetCharm(c *gc.C) {
	charm := s.AddTestingCharm(c, "lxd-profile-fail")
	app := s.AddTestingApplication(c, "lxd-profile-fail", charm)

	c.Assert(charm.LXDProfile(), gc.NotNil)

	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.LXDProfile(), gc.DeepEquals, ch.LXDProfile())

	url, force := app.CharmURL()
	c.Assert(*url, gc.DeepEquals, charm.URL())
	c.Assert(force, jc.IsFalse)

	sch := s.AddMetaCharm(c, "lxd-profile-fail", lxdProfileMetaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		Force:       true,
		ForceUnits:  true,
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err = app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	url, force = app.CharmURL()
	c.Assert(*url, gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	c.Assert(charm.LXDProfile(), gc.DeepEquals, ch.LXDProfile())
}

func (s *ApplicationSuite) TestCAASSetCharm(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch})

	// Add a compatible charm and force it.
	sch := state.AddCustomCharm(c, st, "mysql", "metadata.yaml", metaBaseCAAS, "kubernetes", 2)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(ch.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
}

func (s *ApplicationSuite) TestCAASSetCharmRequireNoUnits(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch, DesiredScale: 1})

	// Add a compatible charm and force it.
	sch := state.AddCustomCharm(c, st, "mysql", "metadata.yaml", metaBaseCAAS, "kubernetes", 2)

	cfg := state.SetCharmConfig{
		Charm:          sch,
		CharmOrigin:    defaultCharmOrigin(ch.URL()),
		ForceUnits:     true,
		RequireNoUnits: true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `.*application should not have units`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentFails(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: gitlab
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  type: stateful
  service: loadbalancer
`[1:]
	newCh := state.AddCustomCharm(c, st, "gitlab", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "gitlab" to charm "local:kubernetes/kubernetes-gitlab-2": cannot change a charm's deployment info`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentTypeFails(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "elastic-operator", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: elastic-operator
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  type: stateful
  service: loadbalancer
`[1:]
	newCh := state.AddCustomCharm(c, st, "elastic-operator", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "elastic-operator" to charm "local:kubernetes/kubernetes-elastic-operator-2": cannot change a charm's deployment type`)
}

func (s *ApplicationSuite) TestCAASSetCharmNewDeploymentModeFails(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "elastic-operator", Charm: ch})

	// Create a charm with new deployment info in metadata.
	metaYaml := `
name: elastic-operator
summary: test
description: test
provides:
  website:
    interface: http
requires:
  db:
    interface: mysql
series:
  - kubernetes
deployment:
  mode: workload
`[1:]
	newCh := state.AddCustomCharm(c, st, "elastic-operator", "metadata.yaml", metaYaml, "kubernetes", 2)
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "elastic-operator" to charm "local:kubernetes/kubernetes-elastic-operator-2": cannot change a charm's deployment mode`)
}

func (s *ApplicationSuite) TestSetCharmWithNewBindings(c *gc.C) {
	sp := s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")
	sch := s.AddMetaCharm(c, "mysql", metaBaseWithNewEndpoint, 2)

	// Assign new charm endpoint to "isolated" space
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
		EndpointBindings: map[string]string{
			"events": sp.Name(),
		},
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	expBindings := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		"events":  sp.Id(),
	}

	updatedBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), gc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestMergeBindings(c *gc.C) {
	s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")

	expBindings := map[string]string{
		"":               network.AlphaSpaceName,
		"metrics-client": network.AlphaSpaceName,
		"server":         network.AlphaSpaceName,
		"server-admin":   network.AlphaSpaceName,
		"db-router":      network.AlphaSpaceName,
	}
	b, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)

	allSpaceInfosLookup, err := s.State.AllSpaceInfos()
	c.Assert(err, jc.ErrorIsNil)

	curBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curBindings, gc.DeepEquals, expBindings)

	// Use MergeBindings to bind "server" -> "isolated"
	b, err = state.NewBindings(s.State, map[string]string{
		"server": "isolated",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.MergeBindings(b, false)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the bindings have been updated
	expBindings["server"] = "isolated"
	b, err = s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	updatedBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings, gc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestMergeBindingsWithForce(c *gc.C) {
	s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")

	sn, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.99.99.0/24"})
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddSpace("far", "", []string{sn.ID()}, false)
	c.Assert(err, gc.IsNil)

	expBindings := map[string]string{
		"":               network.AlphaSpaceName,
		"metrics-client": network.AlphaSpaceName,
		"server":         network.AlphaSpaceName,
		"server-admin":   network.AlphaSpaceName,
		"db-router":      network.AlphaSpaceName,
	}
	b, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)

	allSpaceInfosLookup, err := s.State.AllSpaceInfos()
	c.Assert(err, jc.ErrorIsNil)

	curBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curBindings, gc.DeepEquals, expBindings)

	// Use MergeBindings to force-bind "server" -> "far"
	b, err = state.NewBindings(s.State, map[string]string{
		"server": "far",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.MergeBindings(b, true)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the bindings have been updated
	expBindings["server"] = "far"
	b, err = s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	updatedBindings, err := b.MapWithSpaceNames(allSpaceInfosLookup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings, gc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) TestSetCharmWithNewBindingsAssigneToDefaultSpace(c *gc.C) {
	_ = s.assignUnitOnMachineWithSpaceToApplication(c, s.mysql, "isolated")
	sch := s.AddMetaCharm(c, "mysql", metaBaseWithNewEndpoint, 2)

	// New charm endpoint should be auto-assigned to default space if not
	// explicitly bound by the operator.
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	expBindings := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		"events":  network.AlphaSpaceId,
	}

	updatedBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), gc.DeepEquals, expBindings)
}

func (s *ApplicationSuite) assignUnitOnMachineWithSpaceToApplication(c *gc.C, a *state.Application, spaceName string) *state.Space {
	sn1, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.0.254.0/24"})
	c.Assert(err, gc.IsNil)

	sp, err := s.State.AddSpace(spaceName, "", []string{sn1.ID()}, false)
	c.Assert(err, gc.IsNil)

	m1, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: constraints.MustParse("spaces=isolated"),
	})
	c.Assert(err, gc.IsNil)
	err = m1.SetLinkLayerDevices(state.LinkLayerDeviceArgs{
		Name: "enp5s0",
		Type: network.EthernetDevice,
	})
	c.Assert(err, gc.IsNil)
	err = m1.SetDevicesAddresses(state.LinkLayerDeviceAddress{
		DeviceName:   "enp5s0",
		CIDRAddress:  "10.0.254.42/24",
		ConfigMethod: network.ConfigStatic,
	})
	c.Assert(err, gc.IsNil)

	u1, err := a.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, gc.IsNil)

	return sp
}

func (s *ApplicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *ApplicationSuite) TestSetCharmCharmSettings(c *gc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{"key": "value"}))

	newCh = s.AddConfigCharm(c, "mysql", newStringConfig, 3)
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"other": "one"},
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err = s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key":   "value",
		"other": "one",
	}))
}

func (s *ApplicationSuite) TestSetCharmCharmSettingsForBranch(c *gc.C) {
	c.Assert(s.State.AddBranch("new-branch", "branch-user"), jc.ErrorIsNil)

	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)

	// Update the next generation settings.
	cfg["key"] = "next-gen-value"
	c.Assert(s.mysql.UpdateCharmConfig("new-branch", cfg), jc.ErrorIsNil)

	// Settings for the next generation reflect the change.
	cfg, err = s.mysql.CharmConfig("new-branch")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key": "next-gen-value",
	}))

	// Settings for the current generation are as set with charm.
	cfg, err = s.mysql.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key": "value",
	}))
}

func (s *ApplicationSuite) TestSetCharmCharmSettingsInvalid(c *gc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		CharmOrigin:    defaultCharmOrigin(newCh.URL()),
		ConfigSettings: charm.Settings{"key": 123.45},
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-mysql-2": validating config settings: option "key" expected string, got 123.45`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeries(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "ch:multi-series2-8": base "ubuntu@12.04" not supported by charm, the charm supported bases are: ubuntu@14.04, ubuntu@15.10`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeriesForce(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
		ForceBase:   true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	app, err = s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err = app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.Equals, "ch:multi-series2-8")
}

func (s *ApplicationSuite) TestClientApplicationSetCharmWrongOS(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("12.04"), "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-centos")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		CharmOrigin: defaultCharmOrigin(chDifferentSeries.URL()),
		ForceBase:   true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "ch:multi-series-centos-1": OS "ubuntu" not supported by charm.*`)
}

func (s *ApplicationSuite) TestSetCharmPreconditions(c *gc.C) {
	logging := s.AddTestingCharm(c, "logging")
	cfg := state.SetCharmConfig{
		Charm:       logging,
		CharmOrigin: defaultCharmOrigin(logging.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-logging-1": cannot change an application's subordinacy`)
}

func (s *ApplicationSuite) TestSetCharmUpdatesBindings(c *gc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	clientSpace, err := s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	oldCharm := s.AddMetaCharm(c, "mysql", metaBase, 44)

	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: oldCharm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
		EndpointBindings: map[string]string{
			"":       dbSpace.Id(),
			"server": dbSpace.Id(),
			"client": clientSpace.Id(),
		}})
	c.Assert(err, jc.ErrorIsNil)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)
	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	updatedBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings.Map(), jc.DeepEquals, map[string]string{
		// Existing bindings are preserved.
		"":        dbSpace.Id(),
		"server":  dbSpace.Id(),
		"client":  clientSpace.Id(),
		"cluster": dbSpace.Id(), // inherited from defaults in AddApplication.
		// New endpoints use defaults.
		"foo":  dbSpace.Id(),
		"baz":  dbSpace.Id(),
		"just": dbSpace.Id(),
	})
}

var metaRelationConsumer = `
name: sqlvampire
summary: "connects to an sql server"
description: "lorem ipsum"
requires:
  server: mysql
`
var metaBase = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaBaseCAAS = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaBaseWithNewEndpoint = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
extra-bindings:
  events:
`
var metaDifferentProvider = `
name: mysql
description: none
summary: none
provides:
  server: mysql
  kludge: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaDifferentRequirer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  kludge: mysql
peers:
  cluster: mysql
`
var metaDifferentPeer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  client: mysql
peers:
  kludge: mysql
`
var metaRemoveNonPeerRelation = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaExtraEndpoints = `
name: mysql
description: none
summary: none
provides:
  server: mysql
  foo: bar
requires:
  client: mysql
  baz: woot
peers:
  cluster: mysql
  just: me
`
var lxdProfileMetaBase = `
name: lxd-profile
summary: "Fake LXDProfile"
description: "Fake description"
`

var setCharmEndpointsTests = []struct {
	summary string
	meta    string
	err     string
}{{
	summary: "different provider (but no relation yet)",
	meta:    metaDifferentProvider,
}, {
	summary: "different requirer (but no relation yet)",
	meta:    metaDifferentRequirer,
}, {
	summary: "different peer",
	meta:    metaDifferentPeer,
}, {
	summary: "attempt to break existing non-peer relations",
	meta:    metaRemoveNonPeerRelation,
	err:     `.*would break relation "fakeother:server fakemysql:server"`,
}, {
	summary: "same relations ok",
	meta:    metaBase,
}, {
	summary: "extra endpoints ok",
	meta:    metaExtraEndpoints,
}}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithoutRelations(c *gc.C) {
	revno := 2
	ms := s.AddMetaCharm(c, "mysql", metaBase, revno)
	app := s.AddTestingApplication(c, "fakemysql", ms)
	appServerEP, err := app.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	otherCharm := s.AddMetaCharm(c, "dummy", metaRelationConsumer, 42)
	otherApp := s.AddTestingApplication(c, "fakeother", otherCharm)
	otherServerEP, err := otherApp.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	// Add two mysql units so that peer relations get established and we
	// can check that we are allowed to break them when we upgrade.
	_, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit for the other application and establish a relation.
	_, err = otherApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(appServerEP, otherServerEP)
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       ms,
		CharmOrigin: defaultCharmOrigin(ms.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range setCharmEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)

		newCh := s.AddMetaCharm(c, "mysql", t.meta, revno+i+1)
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithRelations(c *gc.C) {
	revno := 2
	providerCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, revno)
	providerApp := s.AddTestingApplication(c, "myprovider", providerCharm)

	cfg := state.SetCharmConfig{
		Charm:       providerCharm,
		CharmOrigin: defaultCharmOrigin(providerCharm.URL()),
	}
	err := providerApp.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	requirerCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	requirerApp := s.AddTestingApplication(c, "myrequirer", requirerCharm)
	cfg = state.SetCharmConfig{Charm: requirerCharm, CharmOrigin: defaultCharmOrigin(requirerCharm.URL())}
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("myprovider:kludge", "myrequirer:kludge")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	baseCharm := s.AddMetaCharm(c, "mysql", metaBase, revno)
	cfg = state.SetCharmConfig{
		Charm:       baseCharm,
		CharmOrigin: defaultCharmOrigin(baseCharm.URL()),
	}
	err = providerApp.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "myprovider" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "myrequirer" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
}

var stringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
`
var emptyConfig = `
options: {}
`
var floatConfig = `
options:
  key: {default: 0.42, description: Float key, type: float}
`
var newStringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
  other: {default: None, description: My Other, type: string}
`

var sortableConfig = `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
  alphabetic:
    type: int
    description: Something early in the alphabet.
  zygomatic:
    type: int
    description: Something late in the alphabet.
`

var wordpressConfig = `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
`

var setCharmConfigTests = []struct {
	summary     string
	startconfig string
	startvalues charm.Settings
	endconfig   string
	endvalues   charm.Settings
	err         string
}{{
	summary:     "add float key to empty config",
	startconfig: emptyConfig,
	endconfig:   floatConfig,
}, {
	summary:     "add string key to empty config",
	startconfig: emptyConfig,
	endconfig:   stringConfig,
}, {
	summary:     "add string key and preserve existing values",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "foo"},
	endconfig:   newStringConfig,
	endvalues:   charm.Settings{"key": "foo"},
}, {
	summary:     "remove string key",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "value"},
	endconfig:   emptyConfig,
}, {
	summary:     "remove float key",
	startconfig: floatConfig,
	startvalues: charm.Settings{"key": 123.45},
	endconfig:   emptyConfig,
}, {
	summary:     "change key type without values",
	startconfig: stringConfig,
	endconfig:   floatConfig,
}, {
	summary:     "change key type with values",
	startconfig: stringConfig,
	startvalues: charm.Settings{"key": "value"},
	endconfig:   floatConfig,
}}

func (s *ApplicationSuite) TestSetCharmConfig(c *gc.C) {
	charms := map[string]*state.Charm{
		stringConfig:    s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:     s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:     s.AddConfigCharm(c, "wordpress", floatConfig, 3),
		newStringConfig: s.AddConfigCharm(c, "wordpress", newStringConfig, 4),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		app := s.AddTestingApplication(c, "wordpress", origCh)
		err := app.UpdateCharmConfig(model.GenerationMaster, t.startvalues)
		c.Assert(err, jc.ErrorIsNil)

		newCh := charms[t.endconfig]
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		var expectVals charm.Settings
		var expectCh *state.Charm
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			expectCh = origCh
			expectVals = t.startvalues
		} else {
			c.Assert(err, jc.ErrorIsNil)
			expectCh = newCh
			expectVals = t.endvalues
		}

		sch, _, err := app.Charm()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sch.URL(), gc.DeepEquals, expectCh.URL())

		chConfig, err := app.CharmConfig(model.GenerationMaster)
		c.Assert(err, jc.ErrorIsNil)
		expected := s.combinedSettings(sch, expectVals)
		if len(expected) == 0 {
			c.Assert(chConfig, gc.HasLen, 0)
		} else {
			c.Assert(chConfig, gc.DeepEquals, expected)
		}

		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestSetCharmWithDyingApplication(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSequenceUnitIdsAfterDestroy(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestAssignUnitsRemovedAfterAppDestroy(c *gc.C) {
	mariadb := s.AddTestingApplicationWithNumUnits(c, 1, "mariadb", s.charm)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	units, err := mariadb.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	unit := units[0]
	c.Assert(unit.Name(), gc.Equals, "mariadb/0")
	unitAssignments, err := s.State.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 1)

	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = mariadb.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, mariadb)

	unitAssignments, err = s.State.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 0)
}

func (s *ApplicationSuite) TestSequenceUnitIdsAfterDestroyForSidecarApplication(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	s.AddCleanup(func(*gc.C) { _ = st.Close() })
	f := factory.NewFactory(st, s.StatePool)
	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "cockroachdb/0")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, st.ModelUUID())
	assertCleanupCount(c, st, 2)
	unitAssignments, err := st.AllUnitAssignments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(unitAssignments), gc.Equals, 0)

	ch = state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	app = f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
	unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "cockroachdb/0")
}

func (s *ApplicationSuite) TestSequenceUnitIds(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestExplicitUnitName(c *gc.C) {
	name1 := "mysql/100"
	unit, err := s.mysql.AddUnit(state.AddUnitParams{
		UnitName: &name1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, name1)
	name0 := "mysql/0"
	unit, err = s.mysql.AddUnit(state.AddUnitParams{
		UnitName: &name0,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, name0)
}

func (s *ApplicationSuite) TestSetCharmWhenDead(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)

		// Change the application life to Dead manually, as there's no
		// direct way of doing that otherwise.
		ops := []txn.Op{{
			C:      state.ApplicationsC,
			Id:     state.DocID(s.State, s.mysql.Name()),
			Update: bson.D{{"$set", bson.D{{"life", state.Dead}}}},
		}}

		state.RunTransaction(c, s.State, ops)
		assertLife(c, s.mysql, state.Dead)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(errors.Cause(err), gc.Equals, stateerrors.ErrDead)
}

func (s *ApplicationSuite) TestSetCharmWithRemovedApplication(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenRemoved(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertRemoved(c, s.mysql)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenDyingIsOK(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
}

func (s *ApplicationSuite) TestSetCharmRetriesWithSameCharmURL(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, s.charm.URL())

				cfg := state.SetCharmConfig{
					Charm:       sch,
					CharmOrigin: defaultCharmOrigin(sch.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the before hook worked.
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, sch.URL())
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a retry.
			After: func() {
				// Verify it worked after the retry.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsTrue)
				c.Assert(currentCh.URL(), jc.DeepEquals, sch.URL())
			},
		},
	).Check()

	cfg := state.SetCharmConfig{
		Charm:       sch,
		CharmOrigin: defaultCharmOrigin(sch.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)
	cfg := state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State,
		func() {
			err := s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value"})
			c.Assert(err, jc.ErrorIsNil)
		},
		nil, // Ensure there will be a retry.
	).Check()

	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenBothOldAndNewSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add two units, which will keep the refcount of oldCh
				// and newCh settings greater than 0, while the application's
				// charm URLs change between oldCh and newCh. Ensure
				// refcounts change as expected.
				unit1, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				unit2, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				cfg := state.SetCharmConfig{
					Charm:       newCh,
					CharmOrigin: defaultCharmOrigin(newCh.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				err = unit1.SetCharmURL(newCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				// Update newCh settings, switch to oldCh and update its
				// settings as well.
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value1"})
				c.Assert(err, jc.ErrorIsNil)
				cfg = state.SetCharmConfig{
					Charm:       oldCh,
					CharmOrigin: defaultCharmOrigin(oldCh.URL()),
				}

				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = unit2.SetCharmURL(oldCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value2"})
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the second attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: func() {
				// SetCharm has refreshed its cached settings for oldCh
				// and newCh. Change them again to trigger another
				// attempt.
				cfg := state.SetCharmConfig{
					Charm:       newCh,
					CharmOrigin: defaultCharmOrigin(newCh.URL()),
				}

				err := s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value3"})
				c.Assert(err, jc.ErrorIsNil)

				cfg = state.SetCharmConfig{
					Charm:       oldCh,
					CharmOrigin: defaultCharmOrigin(oldCh.URL()),
				}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value4"})
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify the charm and refcounts after the final third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsTrue)
				c.Assert(currentCh.URL(), jc.DeepEquals, newCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
			},
		},
	).Check()

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldBindingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	mysqlKey := state.ApplicationGlobalKey(s.mysql.Name())
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, revno+1)

	cfg := state.SetCharmConfig{
		Charm:       oldCharm,
		CharmOrigin: defaultCharmOrigin(oldCharm.URL()),
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	oldBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(oldBindings.Map(), jc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  network.AlphaSpaceId,
		"kludge":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
	})
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	adminSpace, err := s.State.AddSpace("admin", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	updateBindings := func(updatesMap bson.M) {
		ops := []txn.Op{{
			C:      state.EndpointBindingsC,
			Id:     mysqlKey,
			Update: bson.D{{"$set", updatesMap}},
		}}
		state.RunTransaction(c, s.State, ops)
	}

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// First change.
				updateBindings(bson.M{
					"bindings.server": dbSpace.Id(),
					"bindings.kludge": adminSpace.Id(), // will be removed before newCharm is set.
				})
			},
			After: func() {
				// Second change.
				updateBindings(bson.M{
					"bindings.cluster": adminSpace.Id(),
				})
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify final bindings.
				newBindings, err := s.mysql.EndpointBindings()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(newBindings.Map(), jc.DeepEquals, map[string]string{
					"":        network.AlphaSpaceId,
					"server":  dbSpace.Id(), // from the first change.
					"foo":     network.AlphaSpaceId,
					"client":  network.AlphaSpaceId,
					"baz":     network.AlphaSpaceId,
					"cluster": adminSpace.Id(), // from the second change.
					"just":    network.AlphaSpaceId,
				})
			},
		},
	).Check()

	cfg = state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmViolatesMaxRelationCount(c *gc.C) {
	wp0Charm := `
name: wordpress
description: foo
summary: foo
requires:
  db:
    interface: mysql
    limit: 99
`

	wp1Charm := `
name: wordpress
description: bar
summary: foo
requires:
  db:
    interface: mysql
    limit: 1
`

	// wp0Charm allows up to 99 relations for the db endpoint
	wpApp := s.AddTestingApplication(c, "wordpress", s.AddMetaCharm(c, "wordpress", wp0Charm, 1))

	// Establish 2 relations (note: mysql is already added by the suite setup code)
	s.AddTestingApplication(c, "some-mariadb", s.AddTestingCharm(c, "mariadb"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	eps, err = s.State.InferEndpoints("wordpress", "some-mariadb")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Try to update wordpress to a new version with max 1 relation for the db endpoint
	wpCharmWithRelLimit := s.AddMetaCharm(c, "wordpress", wp1Charm, 2)
	cfg := state.SetCharmConfig{
		Charm:       wpCharmWithRelLimit,
		CharmOrigin: defaultCharmOrigin(wpCharmWithRelLimit.URL()),
	}
	err = wpApp.SetCharm(cfg)
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded, gc.Commentf("expected quota limit error due to max relation mismatch"))

	// Try again with --force
	cfg.Force = true
	err = wpApp.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHash(c *gc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source: "charm-hub",
	})
	err := s.mysql.SetDownloadedIDAndHash("testing-ID", "testing-hash")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().ID, gc.Equals, "testing-ID")
	c.Assert(s.mysql.CharmOrigin().Hash, gc.Equals, "testing-hash")
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashFailEmptyStrings(c *gc.C) {
	err := s.mysql.SetDownloadedIDAndHash("", "")
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashFailChangeID(c *gc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source:   "charm-hub",
		ID:       "testing-ID",
		Hash:     "testing-hash",
		Platform: &state.Platform{},
	})
	err := s.mysql.SetDownloadedIDAndHash("change-ID", "testing-hash")
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *ApplicationSuite) TestSetDownloadedIDAndHashReplaceHash(c *gc.C) {
	s.setupSetDownloadedIDAndHash(c, &state.CharmOrigin{
		Source: "charm-hub",
		ID:     "testing-ID",
		Hash:   "testing-hash",
	})
	err := s.mysql.SetDownloadedIDAndHash("", "new-testing-hash")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmOrigin().Hash, gc.Equals, "new-testing-hash")
}

func (s *ApplicationSuite) setupSetDownloadedIDAndHash(c *gc.C, origin *state.CharmOrigin) {
	origin.Platform = &state.Platform{}
	chInfoOne := s.dummyCharm(c, "ch:testing-3")
	chOne, err := s.State.AddCharm(chInfoOne)
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       chOne,
		CharmOrigin: origin,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
}

var applicationUpdateCharmConfigTests = []struct {
	about   string
	initial charm.Settings
	update  charm.Settings
	expect  charm.Settings
	err     string
}{{
	about:  "unknown option",
	update: charm.Settings{"foo": "bar"},
	err:    `unknown option "foo"`,
}, {
	about:  "bad type",
	update: charm.Settings{"skill-level": "profound"},
	err:    `option "skill-level" expected int, got "profound"`,
}, {
	about:  "set string",
	update: charm.Settings{"outlook": "positive"},
	expect: charm.Settings{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": nil, "title": "sir"},
	expect:  charm.Settings{"title": "sir"},
}, {
	about:  "unset missing string",
	update: charm.Settings{"outlook": nil},
}, {
	about:   `empty strings are valid`,
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": "", "title": ""},
	expect:  charm.Settings{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: charm.Settings{"title": "sir"},
	update:  charm.Settings{"username": "admin001"},
	expect:  charm.Settings{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: charm.Settings{"username": "admin001", "title": "sir"},
	update:  charm.Settings{"username": nil, "title": "My Title"},
	expect:  charm.Settings{"title": "My Title"},
}, {
	about:  "non-string type",
	update: charm.Settings{"skill-level": 303},
	expect: charm.Settings{"skill-level": int64(303)},
}, {
	about:   "unset non-string type",
	initial: charm.Settings{"skill-level": 303},
	update:  charm.Settings{"skill-level": nil},
}}

func (s *ApplicationSuite) TestUpdateCharmConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range applicationUpdateCharmConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateCharmConfig(model.GenerationMaster, t.initial)
			c.Assert(err, jc.ErrorIsNil)
		}
		err := app.UpdateCharmConfig(model.GenerationMaster, t.update)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			cfg, err := app.CharmConfig(model.GenerationMaster)
			c.Assert(err, jc.ErrorIsNil)
			appConfig := t.expect
			expected := s.combinedSettings(sch, appConfig)
			c.Assert(cfg, gc.DeepEquals, expected)
		}
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) setupCharmForTestUpdateApplicationBase(c *gc.C, name string) *state.Application {
	ch := state.AddTestingCharmMultiSeries(c, s.State, name)
	app := state.AddTestingApplicationForBase(c, s.State, state.UbuntuBase("20.04"), name, ch)

	rev := ch.Revision()
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: &rev,
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		},
	}
	cfg := state.SetCharmConfig{
		Charm:       ch,
		CharmOrigin: origin,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func (s *ApplicationSuite) TestUpdateApplicationBase(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesToStart(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	err := app.UpdateApplicationBase(state.UbuntuBase("20.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesAfterStart(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				err = unit.AssignToNewMachine()
				c.Assert(err, jc.ErrorIsNil)

				ops := []txn.Op{{
					C:  state.ApplicationsC,
					Id: state.DocID(s.State, "multi-series"),
					Update: bson.D{{"$set", bson.D{{
						"charm-origin.platform.channel", "22.04/stable"}}}},
				}}
				state.RunTransaction(c, s.State, ops)
			},
			After: func() {
				assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
			},
		},
	).Check()

	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesFail(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				cfg := state.SetCharmConfig{
					Charm:       v2,
					CharmOrigin: defaultCharmOrigin(v2.URL()),
				}
				err := app.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	// Trusty is listed in only version 1 of the charm.
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, gc.ErrorMatches,
		"updating application base: base \"ubuntu@22.04\" not supported by charm, "+
			"the charm supported bases are: ubuntu@20.04, ubuntu@18.04")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesPass(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				origin := defaultCharmOrigin(v2.URL())
				origin.Platform.OS = "ubuntu"
				origin.Platform.Channel = "18.04/stable"
				cfg := state.SetCharmConfig{
					Charm:       v2,
					CharmOrigin: origin,
				}
				err := app.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	// bionic is listed in both revisions of the charm.
	err := app.UpdateApplicationBase(state.UbuntuBase("18.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("18.04"))
}

func (s *ApplicationSuite) setupMultiSeriesUnitSubordinate(c *gc.C, app *state.Application, name string) *state.Application {
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	return s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, name)
}

func (s *ApplicationSuite) setupMultiSeriesUnitSubordinateGivenUnit(c *gc.C, app *state.Application, unit *state.Unit, name string) *state.Application {
	subApp := s.setupCharmForTestUpdateApplicationBase(c, name)

	eps, err := s.State.InferEndpoints(app.Name(), name)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = subApp.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	return subApp
}

func assertApplicationBaseUpdate(c *gc.C, a *state.Application, base state.Base) {
	err := a.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	stBase, err := corebase.ParseBase(base.OS, base.Channel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.Base().String(), gc.Equals, stBase.String())
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinate(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateFail(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("16.04"), false)
	c.Assert(err, gc.ErrorMatches, `updating application base: base "ubuntu@16.04" not supported by charm.*`)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("20.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateForce(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	err := app.UpdateApplicationBase(state.UbuntuBase("16.04"), true)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("16.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("16.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesUnitCountChange(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 0)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add a subordinate and unit
				_ = s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))

	units, err = app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	subApp, err := s.State.Application("multi-series-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinate(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), gc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				_ = s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, "multi-series-subordinate2")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("22.04"), false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("22.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("22.04"))

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp2, state.UbuntuBase("22.04"))
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinateIncompatible(c *gc.C) {
	app := s.setupCharmForTestUpdateApplicationBase(c, "multi-series")
	subApp := s.setupMultiSeriesUnitSubordinate(c, app, "multi-series-subordinate")
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), gc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				_ = s.setupMultiSeriesUnitSubordinateGivenUnit(c, app, unit, "multi-series-subordinate2")
			},
		},
	).Check()

	err = app.UpdateApplicationBase(state.UbuntuBase("18.04"), false)
	c.Assert(err, gc.ErrorMatches, `updating application base: base "ubuntu@18.04" not supported by charm.*`)
	assertApplicationBaseUpdate(c, app, state.UbuntuBase("20.04"))
	assertApplicationBaseUpdate(c, subApp, state.UbuntuBase("20.04"))

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationBaseUpdate(c, subApp2, state.UbuntuBase("20.04"))
}

func assertNoSettingsRef(c *gc.C, st *state.State, appName string, sch *state.Charm) {
	cURL := sch.URL()
	_, err := state.ApplicationSettingsRefCount(st, appName, &cURL)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotFound)
}

func assertSettingsRef(c *gc.C, st *state.State, appName string, sch *state.Charm, refcount int) {
	cURL := sch.URL()
	rc, err := state.ApplicationSettingsRefCount(st, appName, &cURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, refcount)
}

func (s *ApplicationSuite) TestSettingsRefCountWorks(c *gc.C) {
	// This test ensures the application settings per charm URL are
	// properly reference counted.
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	// Both refcounts are zero initially.
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// app is using oldCh, so its settings refcount is incremented.
	app := s.AddTestingApplication(c, appName, oldCh)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing to the same charm does not change the refcount.
	cfg := state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing from oldCh to newCh causes the refcount of oldCh's
	// settings to be decremented, while newCh's settings is
	// incremented. Consequently, because oldCh's refcount is 0, the
	// settings doc will be removed.
	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertSettingsRef(c, s.State, appName, newCh, 1)

	// The same but newCh swapped with oldCh.
	cfg = state.SetCharmConfig{
		Charm:       oldCh,
		CharmOrigin: defaultCharmOrigin(oldCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Adding a unit without a charm URL set does not affect the
	// refcount.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	charmURL := u.CharmURL()
	c.Assert(charmURL, gc.IsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Setting oldCh as the units charm URL increments oldCh, which is
	// used by app as well, hence 2.
	err = u.SetCharmURL(oldCh.URL())
	c.Assert(err, jc.ErrorIsNil)
	charmURL = u.CharmURL()
	c.Assert(charmURL, gc.NotNil)
	c.Assert(*charmURL, gc.Equals, oldCh.URL())
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// A dead unit does not decrement the refcount.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Once the unit is removed, refcount is decremented.
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Finally, after the application is destroyed and removed (since the
	// last unit's gone), the refcount is again decremented.
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Having studiously avoided triggering cleanups throughout,
	// invoke them now and check that the charms are cleaned up
	// correctly -- and that a storm of cleanups for the same
	// charm are not a problem.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)
	err = oldCh.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = newCh.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSettingsRefCreateRace(c *gc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// just before setting the unit charm url, switch the application
	// away from the original charm, causing the attempt to fail
	// (because the settings have gone away; it's the unit's job to
	// fail out and handle the new charm when it comes back up
	// again).
	dropSettings := func() {
		cfg := state.SetCharmConfig{
			Charm:       newCh,
			CharmOrigin: defaultCharmOrigin(newCh.URL()),
		}
		err = app.SetCharm(cfg)
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, dropSettings).Check()

	err = unit.SetCharmURL(oldCh.URL())
	c.Check(err, gc.ErrorMatches, "settings reference: does not exist")
}

func (s *ApplicationSuite) TestSettingsRefRemoveRace(c *gc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// just before updating the app charm url, set that charm url on
	// a unit to block the removal.
	grabReference := func() {
		err := unit.SetCharmURL(oldCh.URL())
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, grabReference).Check()

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// check refs to both settings exist
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertSettingsRef(c, s.State, appName, newCh, 1)
}

func assertNoOffersRef(c *gc.C, st *state.State, appName string) {
	_, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotFound)
}

func assertOffersRef(c *gc.C, st *state.State, appName string, refcount int) {
	rc, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, refcount)
}

func (s *ApplicationSuite) TestOffersRefCountWorks(c *gc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	// Once the offer is removed, refcount is decremented.
	err = ao.Remove("hosted-mysql", false)
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	// Trying to destroy the app while there is an offer
	// succeeds when that offer has no connections
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *ApplicationSuite) TestDestroyApplicationRemoveOffers(c *gc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")

	offers, err := ao.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestOffersRefRace(c *gc.C) {
	addOffer := func() {
		ao := state.NewApplicationOffers(s.State)
		_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.Owner.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *ApplicationSuite) TestOffersRefRaceWithForce(c *gc.C) {
	addOffer := func() {
		ao := state.NewApplicationOffers(s.State)
		_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.Owner.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()

	op := s.mysql.DestroyOperation()
	op.Force = true
	err := s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	assertNoOffersRef(c, s.State, "mysql")
}

const mysqlBaseMeta = `
name: mysql
summary: "Database engine"
description: "A pretty popular database"
provides:
  server: mysql
`
const onePeerMeta = `
peers:
  cluster: mysql
`
const onePeerAltMeta = `
peers:
  minion: helper
`
const twoPeersMeta = `
peers:
  cluster: mysql
  loadbalancer: phony
`

func (s *ApplicationSuite) assertApplicationRelations(c *gc.C, app *state.Application, expectedKeys ...string) []*state.Relation {
	rels, err := app.Relations()
	c.Assert(err, jc.ErrorIsNil)
	if len(rels) == 0 {
		return nil
	}
	relKeys := make([]string, len(expectedKeys))
	for i, rel := range rels {
		relKeys[i] = rel.String()
	}
	sort.Strings(relKeys)
	c.Assert(relKeys, gc.DeepEquals, expectedKeys)
	return rels
}

func (s *ApplicationSuite) TestNewPeerRelationsAddedOnUpgrade(c *gc.C) {
	// Original mysql charm has no peer relations.
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoPeersMeta, 3)

	// No relations joined yet.
	s.assertApplicationRelations(c, s.mysql)

	cfg := state.SetCharmConfig{Charm: oldCh, CharmOrigin: defaultCharmOrigin(oldCh.URL())}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:cluster")

	cfg = state.SetCharmConfig{Charm: newCh, CharmOrigin: defaultCharmOrigin(newCh.URL())}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	rels := s.assertApplicationRelations(c, s.mysql, "mysql:cluster", "mysql:loadbalancer")

	// Check state consistency by attempting to destroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func (s *ApplicationSuite) TestStalePeerRelationsRemovedOnUpgrade(c *gc.C) {
	// Original mysql charm has no peer relations.
	// oldCh is mysql + the peer relation "mysql:cluster"
	// newCh is mysql + the peer relation "mysql:minion"
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerAltMeta, 42)

	// No relations joined yet.
	s.assertApplicationRelations(c, s.mysql)

	cfg := state.SetCharmConfig{Charm: oldCh, CharmOrigin: defaultCharmOrigin(oldCh.URL())}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:cluster")

	// Since the two charms have different URLs, the following SetCharm call
	// emulates a "juju refresh --switch" request. We expect that any prior
	// peer relations that are not referenced by the new charm metadata
	// to be dropped and any new peer relations to be created.
	cfg = state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		ForceUnits:  true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	rels := s.assertApplicationRelations(c, s.mysql, "mysql:minion")

	// Check state consistency by attempting to destroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func jujuInfoEp(applicationname string) state.Endpoint {
	return state.Endpoint{
		ApplicationName: applicationname,
		Relation: charm.Relation{
			Interface: "juju-info",
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
}

func (s *ApplicationSuite) TestTag(c *gc.C) {
	c.Assert(s.mysql.Tag().String(), gc.Equals, "application-mysql")
}

func (s *ApplicationSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.mysql.Endpoint("mysql")
	c.Assert(err, gc.ErrorMatches, `application "mysql" has no "mysql" relation`)

	jiEP, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("mysql"))

	serverEP, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	serverAdminEP, err := s.mysql.Endpoint("server-admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverAdminEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "server-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	dbRouterEP, err := s.mysql.Endpoint("db-router")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dbRouterEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "db-router",
			Name:      "db-router",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	monitoringEP, err := s.mysql.Endpoint("metrics-client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(monitoringEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "metrics",
			Name:      "metrics-client",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := s.mysql.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, jc.SameContents, []state.Endpoint{jiEP, serverEP, serverAdminEP, dbRouterEP, monitoringEP})
}

func (s *ApplicationSuite) TestRiakEndpoints(c *gc.C) {
	riak := s.AddTestingApplication(c, "myriak", s.AddTestingCharm(c, "riak"))

	_, err := riak.Endpoint("garble")
	c.Assert(err, gc.ErrorMatches, `application "myriak" has no "garble" relation`)

	jiEP, err := riak.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("myriak"))

	ringEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ringEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "riak",
			Name:      "ring",
			Role:      charm.RolePeer,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEP, err := riak.Endpoint("admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(adminEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	endpointEP, err := riak.Endpoint("endpoint")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpointEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "endpoint",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := riak.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{adminEP, endpointEP, jiEP, ringEP})
}

func (s *ApplicationSuite) TestWordpressEndpoints(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	_, err := wordpress.Endpoint("nonsense")
	c.Assert(err, gc.ErrorMatches, `application "wordpress" has no "nonsense" relation`)

	jiEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("wordpress"))

	urlEP, err := wordpress.Endpoint("url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urlEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "url",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	ldEP, err := wordpress.Endpoint("logging-dir")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ldEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-dir",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	mpEP, err := wordpress.Endpoint("monitoring-port")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mpEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "monitoring",
			Name:      "monitoring-port",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	dbEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dbEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     1,
		},
	})

	cacheEP, err := wordpress.Endpoint("cache")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cacheEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
		Relation: charm.Relation{
			Interface: "varnish",
			Name:      "cache",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     2,
			Optional:  true,
		},
	})

	eps, err := wordpress.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{cacheEP, dbEP, jiEP, ldEP, mpEP, urlEP})
}

func (s *ApplicationSuite) TestApplicationRefresh(c *gc.C) {
	s1, err := s.State.Application(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       s.charm,
		CharmOrigin: defaultCharmOrigin(s.charm.URL()),
		ForceUnits:  true,
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	testch, force, err := s1.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(testch.URL(), gc.DeepEquals, s.charm.URL())

	err = s1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	testch, force, err = s1.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsTrue)
	c.Assert(testch.URL(), gc.DeepEquals, s.charm.URL())

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
}

func (s *ApplicationSuite) TestSetPassword(c *gc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Application(s.mysql.Name())
	})
}

func (s *ApplicationSuite) TestApplicationExposed(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Check that setting and clearing the exposed flag works correctly.
	err := s.mysql.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsTrue)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Check that setting and clearing the exposed flag repeatedly does not fail.
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsTrue)

	// Make the application Dying and check that ClearExposed and MergeExposeSettings fail.
	// TODO(fwereade): maybe application destruction should always unexpose?
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, gc.ErrorMatches, notAliveErr)

	// Remove the application and check that both fail.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.MergeExposeSettings(nil)
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
}

func (s *ApplicationSuite) TestApplicationExposeEndpoints(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Check argument validation
	err := s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"":               {},
		"bogus-endpoint": {},
	})
	c.Assert(err, gc.ErrorMatches, `.*endpoint "bogus-endpoint" not found`)
	err = s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"server": {ExposeToSpaceIDs: []string{"bogus-space-id"}},
	})
	c.Assert(err, gc.ErrorMatches, `.*space with ID "bogus-space-id" not found`)
	err = s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"server": {ExposeToCIDRs: []string{"not-a-cidr"}},
	})
	c.Assert(err, gc.ErrorMatches, `.*unable to parse "not-a-cidr" as a CIDR.*`)

	// Check that the expose parameters are properly persisted
	exp := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err = s.mysql.MergeExposeSettings(exp)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, exp)

	// Refresh model and ensure that we get the same parameters
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, exp)
}

func (s *ApplicationSuite) TestApplicationExposeEndpointMergeLogic(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Set initial value
	initial := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err := s.mysql.MergeExposeSettings(initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, initial)

	// The merge call should overwrite the "server" value and append the
	// entry for "server-admin"
	updated := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToCIDRs: []string{"0.0.0.0/0"},
		},
		"server-admin": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err = s.mysql.MergeExposeSettings(updated)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, updated)
}

func (s *ApplicationSuite) TestApplicationExposeWithoutSpaceAndCIDR(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	err := s.mysql.MergeExposeSettings(map[string]state.ExposedEndpoint{
		// If the expose params are empty, an implicit 0.0.0.0/0 will
		// be assumed (equivalent to: juju expose --endpoints server)
		"server": {},
	})
	c.Assert(err, jc.ErrorIsNil)

	exp := map[string]state.ExposedEndpoint{
		"server": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	}
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, exp, gc.Commentf("expected the implicit 0.0.0.0/0 and ::/0 CIDRs to be added when an empty ExposedEndpoint value is provided to MergeExposeSettings"))
}

func (s *ApplicationSuite) TestApplicationUnsetExposeEndpoints(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Set initial value
	initial := map[string]state.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"13.37.0.0/16"},
		},
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}
	err := s.mysql.MergeExposeSettings(initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, initial)

	// Check argument validation
	err = s.mysql.UnsetExposeSettings([]string{"bogus-endpoint"})
	c.Assert(err, gc.ErrorMatches, `.*endpoint "bogus-endpoint" not found`)
	err = s.mysql.UnsetExposeSettings([]string{"server-admin"})
	c.Assert(err, gc.ErrorMatches, `.*endpoint "server-admin" is not exposed`)

	// Check unexpose logic
	err = s.mysql.UnsetExposeSettings([]string{""})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.DeepEquals, map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	}, gc.Commentf("expected the entry of the wildcard endpoint to be removed"))
	c.Assert(s.mysql.IsExposed(), jc.IsTrue, gc.Commentf("expected application to remain exposed"))

	err = s.mysql.UnsetExposeSettings([]string{"server"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.ExposedEndpoints(), gc.HasLen, 0)
	c.Assert(s.mysql.IsExposed(), jc.IsFalse, gc.Commentf("expected exposed flag to be cleared when last expose setting gets removed"))
}

func (s *ApplicationSuite) TestAddUnit(c *gc.C) {
	// Check that principal units can be added on their own.
	c.Assert(s.mysql.UnitCount(), gc.Equals, 0)
	unitZero, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.UnitCount(), gc.Equals, 1)
	c.Assert(unitZero.Name(), gc.Equals, "mysql/0")
	c.Assert(unitZero.IsPrincipal(), jc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), gc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), gc.Equals, s.State.ModelUUID())

	unitOne, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitOne.Name(), gc.Equals, "mysql/1")
	c.Assert(unitOne.IsPrincipal(), jc.IsTrue)
	c.Assert(unitOne.SubordinateNames(), gc.HasLen, 0)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)

	// Add a subordinate application and check that units cannot be added directly.
	// to add a subordinate unit.
	subCharm := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", subCharm)
	_, err = logging.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "logging": application is a subordinate`)

	// Indirectly create a subordinate unit by adding a relation and entering
	// scope as a principal.
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unitZero)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subZero, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Check that once it's refreshed unitZero has subordinates.
	err = unitZero.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.SubordinateNames(), gc.DeepEquals, []string{"logging/0"})

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, m.Id())
}

func (s *ApplicationSuite) TestAddUnitWhenNotAlive(c *gc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "mysql": application is not found or not alive`)
	c.Assert(u.EnsureDead(), jc.ErrorIsNil)
	c.Assert(u.Remove(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "mysql": application "mysql" not found`)
}

func (s *ApplicationSuite) TestAddCAASUnit(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	unitZero, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.Name(), gc.Equals, "gitlab/0")
	c.Assert(unitZero.IsPrincipal(), jc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), gc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), gc.Equals, st.ModelUUID())

	err = unitZero.SetWorkloadVersion("3.combined")
	c.Assert(err, jc.ErrorIsNil)
	version, err := unitZero.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, "3.combined")

	err = unitZero.SetMeterStatus(state.MeterGreen.String(), "all good")
	c.Assert(err, jc.ErrorIsNil)
	ms, err := unitZero.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms.Code, gc.Equals, state.MeterGreen)
	c.Assert(ms.Info, gc.Equals, "all good")

	// But they do have status.
	us, err := unitZero.Status()
	c.Assert(err, jc.ErrorIsNil)
	us.Since = nil
	c.Assert(us, jc.DeepEquals, status.StatusInfo{
		Status:  status.Waiting,
		Message: status.MessageInstallingAgent,
		Data:    map[string]interface{}{},
	})
	as, err := unitZero.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(as.Since, gc.NotNil)
	as.Since = nil
	c.Assert(as, jc.DeepEquals, status.StatusInfo{
		Status: status.Allocating,
		Data:   map[string]interface{}{},
	})
}

func (s *ApplicationSuite) TestAgentTools(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	agentTools := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: "ubuntu",
	}

	tools, err := app.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools.Version, gc.DeepEquals, agentTools)
}

func (s *ApplicationSuite) TestSetAgentVersion(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	agentVersion := version.MustParseBinary("2.0.1-ubuntu-and64")
	err := app.SetAgentVersion(agentVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	tools, err := app.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools.Version, gc.DeepEquals, agentVersion)
}

func (s *ApplicationSuite) TestAddUnitWithProviderIdNonCAASModel(c *gc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, jc.ErrorIsNil)
	_, err = u.ContainerInfo()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestReadUnit(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that retrieving a unit from state works correctly.
	unit, err := s.State.Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.State.Unit("mysql")
	c.Assert(err, gc.ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.State.Unit("mysql/0/0")
	c.Assert(err, gc.ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.State.Unit("pressword/0")
	c.Assert(err, gc.ErrorMatches, `unit "pressword/0" not found`)

	units, err := s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sortedUnitNames(units), gc.DeepEquals, []string{"mysql/0", "mysql/1"})
}

func (s *ApplicationSuite) TestReadUnitWhenDying(c *gc.C) {
	// Test that we can still read units when the application is Dying...
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	_, err = s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)

	// ...and when those units are Dying or Dead...
	testWhenDying(c, unit, noErr, noErr, func() error {
		_, err := s.mysql.AllUnits()
		return err
	}, func() error {
		_, err := s.State.Unit("mysql/0")
		return err
	})

	// ...and even, in a very limited way, when the application itself is removed.
	removeAllUnits(c, s.mysql)
	_, err = s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroySimple(c *gc.C) {
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyRemovesStatusHistory(c *gc.C) {
	err := s.mysql.SetStatus(status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 100}
	agentInfo, err := s.mysql.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(agentInfo), gc.Equals, 2)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	agentInfo, err = s.mysql.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentInfo, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestDestroyStillHasUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(s.mysql.Refresh(), jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	c.Assert(unit.Remove(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyOnceHadUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyStaleNonZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyStaleZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	c.Assert(unit.Remove(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a application with no units in relation scope; check application and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyWithRemovableApplicationOpenedPortRanges(c *gc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 0)

	unit0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	portRangesUnit0, err := unit0.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	portRangesUnit0.Open(allEndpoints, network.MustParsePortRange("3000/tcp"))
	portRangesUnit0.Open(allEndpoints, network.MustParsePortRange("3001/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit0.Changes()), jc.ErrorIsNil)

	portRangesUnit0, err = unit0.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(portRangesUnit0.UniquePortRanges(), gc.HasLen, 2)

	unit1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	portRangesUnit1, err := unit1.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3001/tcp"))
	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3002/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), jc.ErrorIsNil)

	portRangesUnit1, err = unit1.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(portRangesUnit1.UniquePortRanges(), gc.HasLen, 2)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 3)

	portRangesUnit1.Close(allEndpoints, network.MustParsePortRange("3002/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), jc.ErrorIsNil)

	portRangesUnit1, err = unit1.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(portRangesUnit1.UniquePortRanges(), gc.HasLen, 1)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 2)

	portRangesUnit1.Open(allEndpoints, network.MustParsePortRange("3003/tcp"))
	c.Assert(st.ApplyOperation(portRangesUnit1.Changes()), jc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 3)

	err = unit1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 2)

	// Remove all units, all opened ports should be removed.
	err = unit0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	appPortRanges, err = app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 0)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestOpenedPortRanges(c *gc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	portRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	flush := func(expectedErr string) {
		if len(expectedErr) == 0 {
			c.Assert(st.ApplyOperation(portRanges.Changes()), jc.ErrorIsNil)
		} else {
			c.Assert(st.ApplyOperation(portRanges.Changes()), gc.ErrorMatches, expectedErr)
		}
		portRanges, err = unit.OpenedPortRanges()
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Assert(portRanges.UniquePortRanges(), gc.HasLen, 0)
	portRanges.Open(allEndpoints, network.MustParsePortRange("3000/tcp"))
	portRanges.Open("data-port", network.MustParsePortRange("2000/udp"))
	// All good.
	flush(``)
	c.Assert(portRanges.UnitName(), jc.DeepEquals, `cockroachdb/0`)
	c.Assert(portRanges.UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("3000/tcp"),
		network.MustParsePortRange("2000/udp"),
	})
	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// Errors for unknown endpoint.
	portRanges.Open("bad-endpoint", network.MustParsePortRange("2000/udp"))
	flush(`cannot open/close ports: open port range: endpoint "bad-endpoint" for application "cockroachdb" not found`)
	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// No ops for duplicated Open.
	portRanges.Open("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
		"data-port":  []network.PortRange{network.MustParsePortRange("2000/udp")},
	})

	// Close one port.
	portRanges.Close("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
	})

	// No ops for Close non existing port.
	portRanges.Close("data-port", network.MustParsePortRange("2000/udp"))
	flush(``)
	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("3000/tcp")},
	})

	// Destroy the application; check application and
	// openedApplicationportRanges removed.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 0)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *ApplicationSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err = s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	c.Assert(s.mysql.Destroy(), jc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel0.Life(), gc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are are both removed.
	c.Assert(ru.LeaveScope(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyQueuesUnitCleanup(c *gc.C) {
	// Add 5 units; block quick-remove of mysql/1 and mysql/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			preventUnitDestroyRemove(c, unit)
		}
	}

	s.assertNoCleanup(c)

	// Destroy mysql, and check units are not touched.
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	s.assertNeedsCleanup(c)

	// Run the cleanup and check the units.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)
	for i, unit := range units {
		if i%2 != 0 {
			assertLife(c, unit, state.Dying)
		} else {
			assertRemoved(c, unit)
		}
	}

	// Check for queued unit cleanups, and run them.
	s.assertNeedsCleanup(c)
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestRemoveApplicationMachine(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	c.Assert(s.mysql.Destroy(), gc.IsNil)
	assertLife(c, s.mysql, state.Dying)

	// Application.Destroy adds units to cleanup, make it happen now.
	c.Assert(s.State.Cleanup(fakeSecretDeleter), gc.IsNil)

	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	assertLife(c, machine, state.Dying)
}

func (s *ApplicationSuite) TestDestroyRemoveAlsoDeletesSecretPermissions(c *gc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	// Make a relation for the access scope.
	endpoint1, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "logging",
		}),
	})
	endpoint2, err := application2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint1, endpoint2},
	})

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       rel.Tag(),
		Subject:     s.mysql.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, s.mysql.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.SecretAccess(uri, s.mysql.Tag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyRemoveAlsoDeletesOwnedSecrets(c *gc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = store.GetSecret(uri)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Create again, no label clash.
	s.AddTestingApplication(c, "mysql", s.charm)
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyNoRemoveKeepsOwnedSecrets(c *gc.C) {
	// Create a relation so destroy does not remove.
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wp := s.AddTestingApplication(c, "wordpress", wpch)
	wpep, err := wp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(mysqlep, wpep)
	c.Assert(err, jc.ErrorIsNil)

	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = store.GetSecret(uri)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestApplicationCleanupRemovesStorageConstraints(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetCharmURL(ch.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(app.Destroy(), gc.IsNil)
	assertLife(c, app, state.Dying)
	assertCleanupCount(c, s.State, 2)

	// These next API calls are normally done by the uniter.
	c.Assert(u.EnsureDead(), jc.ErrorIsNil)
	c.Assert(u.Remove(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)

	// Ensure storage constraints and settings are now gone.
	_, err = state.AppStorageConstraints(app)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	cfg := state.GetApplicationCharmConfig(s.State, app)
	err = cfg.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestApplicationCleanupRemovesAppFromActiveBranches(c *gc.C) {
	s.assertNoCleanup(c)

	// setup branch, tracking and app with config changes.
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(s.Model.AddBranch("apple", "testuser"), jc.ErrorIsNil)
	branch, err := s.Model.Branch("apple")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(branch.AssignApplication(app.Name()), jc.ErrorIsNil)
	c.Assert(branch.AssignApplication(s.mysql.Name()), jc.ErrorIsNil)
	newCfg := map[string]interface{}{"outlook": "testing"}
	c.Assert(app.UpdateCharmConfig(branch.BranchName(), newCfg), jc.ErrorIsNil)

	// verify the branch setup
	c.Assert(branch.Refresh(), jc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), jc.DeepEquals, map[string][]string{
		app.Name():     {},
		s.mysql.Name(): {},
	})
	branchCfg := branch.Config()
	_, ok := branchCfg[app.Name()]
	c.Assert(ok, jc.IsTrue)

	// destroy the app
	c.Assert(app.Destroy(), gc.IsNil)
	assertRemoved(c, app)

	// Check the branch
	c.Assert(branch.Refresh(), jc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), jc.DeepEquals, map[string][]string{
		s.mysql.Name(): {},
	})
	c.Assert(branch.Config(), gc.HasLen, 0)
}

func (s *ApplicationSuite) TestRemoveQueuesLocalCharmCleanup(c *gc.C) {
	s.assertNoCleanup(c)

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Check a cleanup doc was added.
	s.assertNeedsCleanup(c)

	// Run the cleanup
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)

	// Check charm removed
	err = s.charm.Refresh()
	c.Check(err, jc.Satisfies, errors.IsNotFound)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestDestroyQueuesResourcesCleanup(c *gc.C) {
	s.assertNoCleanup(c)

	// Add a resource to the application, ensuring it is stored.
	rSt := s.State.Resources()
	const content = "abc"
	res := resourcetesting.NewCharmResource(c, "blob", content)
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res, strings.NewReader(content), state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)
	storagePath := state.ResourceStoragePath(c, s.State, outRes.ID)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// Cleanup should be registered but not yet run.
	s.assertNeedsCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsTrue)

	// Run the cleanup.
	err = s.State.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsFalse)
}

func (s *ApplicationSuite) TestDestroyWithPlaceholderResources(c *gc.C) {
	s.assertNoCleanup(c)

	// Add a placeholder resource to the application.
	rSt := s.State.Resources()
	res := resourcetesting.NewPlaceholderResource(c, "blob", s.mysql.Name())
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res.Resource, nil, state.IncrementCharmModifiedVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outRes.IsPlaceholder(), jc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// No cleanup required for placeholder resources.
	state.AssertNoCleanupsWithKind(c, s.State, "resourceBlob")
}

func (s *ApplicationSuite) TestReadUnitWithChangingState(c *gc.C) {
	// Check that reading a unit after removing the application
	// fails nicely.
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, gc.ErrorMatches, `unit "mysql/0" not found`)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *ApplicationSuite) TestConstraints(c *gc.C) {
	// Constraints are initially empty (for now).
	cons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, gc.Not(jc.Satisfies), constraints.IsEmpty)
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("arch=amd64"))

	// Constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(4096)}
	err = s.mysql.SetConstraints(cons2)
	c.Assert(err, jc.ErrorIsNil)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(750)}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, jc.ErrorIsNil)
	cons5, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons5, gc.DeepEquals, cons4)

	// Destroy the existing application; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// ...but we can check that old constraints do not affect new applications
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons6, gc.Not(jc.Satisfies), constraints.IsEmpty)
	c.Assert(cons6, gc.DeepEquals, constraints.MustParse("arch=amd64"))
}

func (s *ApplicationSuite) TestArchConstraints(c *gc.C) {
	amdArch := "amd64"
	armArch := "arm64"

	cons2 := constraints.Value{Arch: &amdArch}
	err := s.mysql.SetConstraints(cons2)
	c.Assert(err, jc.ErrorIsNil)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Constraints error out if it's already set.
	cons4 := constraints.Value{Arch: &armArch}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, gc.ErrorMatches, "changing architecture \\(amd64\\) not supported")

	// Destroy the existing application; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	// ...but we can check that old constraints do not affect new applications
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(constraints.IsEmpty(&cons6), jc.IsFalse)
	c.Assert(cons6, jc.DeepEquals, cons2)
}

func (s *ApplicationSuite) TestSetInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *ApplicationSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw), gc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting constraints on application "mysql": unsupported constraints: cpu-power`},
	})
	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scons, gc.DeepEquals, cons)
}

func (s *ApplicationSuite) TestConstraintsLifecycle(c *gc.C) {
	// Dying.
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)

	cons1 := constraints.MustParse("mem=1G")
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: application is not found or not alive`)

	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&scons, gc.Not(jc.Satisfies), constraints.IsEmpty)
	c.Assert(scons, gc.DeepEquals, constraints.MustParse("arch=amd64"))

	// Removed (== Dead, for a application).
	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(unit.Remove(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: application is not found or not alive`)
	_, err = s.mysql.Constraints()
	c.Assert(err, gc.ErrorMatches, `constraints not found`)
}

func (s *ApplicationSuite) TestSubordinateConstraints(c *gc.C) {
	loggingCh := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", loggingCh)

	_, err := logging.Constraints()
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)

	err = logging.SetConstraints(constraints.Value{})
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)
}

func (s *ApplicationSuite) TestWatchUnitsBulkEvents(c *gc.C) {
	// Alive unit...
	alive, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Dying unit...
	dying, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dying)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead unit...
	dead, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dead)
	err = dead.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Gone unit.
	gone, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = gone.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// All except gone unit are reported in initial event.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange(alive.Name(), dying.Name(), dead.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *ApplicationSuite) TestWatchUnitsLifecycle(c *gc.C) {
	// Empty initial event when no units.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Create one unit, check one change.
	quick, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Destroy that unit (short-circuited to removal), check one change.
	err = quick.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Create another, check one change.
	slow, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Change unit itself, no change.
	preventUnitDestroyRemove(c, slow)
	wc.AssertNoChange()

	// Make unit Dying, change detected.
	err = slow.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Make unit Dead, change detected.
	err = slow.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Remove unit, final change not detected.
	err = slow.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationSuite) TestWatchRelations(c *gc.C) {
	// TODO(fwereade) split this test up a bit.
	w := s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a relation; check change.
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wpi := 0
	addRelation := func() *state.Relation {
		name := fmt.Sprintf("wp%d", wpi)
		wpi++
		wp := s.AddTestingApplication(c, name, wpch)
		wpep, err := wp.Endpoint("db")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(mysqlep, wpep)
		c.Assert(err, jc.ErrorIsNil)
		return rel
	}
	rel0 := addRelation()
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Add another relation; check change.
	rel1 := addRelation()
	wc.AssertChange(rel1.String())
	wc.AssertNoChange()

	// Destroy a relation; check change.
	err = rel0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Stop watcher; check change chan is closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Add a new relation; start a new watcher; check initial event.
	rel2 := addRelation()
	w = s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, w)
	wc.AssertChange(rel1.String(), rel2.String())
	wc.AssertNoChange()

	// Add a unit to the new relation; check no change.
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru2, err := rel2.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the relation with the unit in scope, and add another; check
	// changes.
	err = rel2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel2.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	rel3 := addRelation()
	wc.AssertChange(rel2.String(), rel3.String())
	wc.AssertNoChange()

	// Leave scope, destroying the relation, and check that change as well.
	err = ru2.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel2.String())
	wc.AssertNoChange()

	// Watch relations on the requirer application too (exercises a
	// different path of the WatchRelations filter function)
	wpx := s.AddTestingApplication(c, "wpx", wpch)
	wpxWatcher := wpx.WatchRelations()
	defer testing.AssertStop(c, wpxWatcher)
	wpxWatcherC := testing.NewStringsWatcherC(c, wpxWatcher)
	wpxWatcherC.AssertChange()
	wpxWatcherC.AssertNoChange()

	wpxep, err := wpx.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	relx, err := s.State.AddRelation(mysqlep, wpxep)
	c.Assert(err, jc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()

	err = relx.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()
}

func removeAllUnits(c *gc.C, s *state.Application) {
	us, err := s.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = u.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestWatchApplication(c *gc.C) {
	w := s.mysql.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	application, err := s.State.Application(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)
	err = application.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = application.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()

	cfg := state.SetCharmConfig{
		Charm:       s.charm,
		CharmOrigin: defaultCharmOrigin(s.charm.URL()),
		ForceUnits:  true,
	}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove application, start new watch, check single event.
	err = application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// The destruction needs to have been processed by the txn watcher before the
	// watcher in the test is started or the destroy notification may come through
	// as an additional event.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = s.mysql.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *ApplicationSuite) TestMetricCredentials(c *gc.C) {
	err := s.mysql.SetMetricCredentials([]byte("hello there"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.MetricCredentials(), gc.DeepEquals, []byte("hello there"))

	application, err := s.State.Application(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.MetricCredentials(), gc.DeepEquals, []byte("hello there"))
}

func (s *ApplicationSuite) TestMetricCredentialsOnDying(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetMetricCredentials([]byte("set before dying"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.SetMetricCredentials([]byte("set after dying"))
	c.Assert(err, gc.ErrorMatches, "cannot update metric credentials: application is not found or not alive")
}

const oneRequiredStorageMeta = `
storage:
  data0:
    type: block
`

const oneOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
`

const oneRequiredOneOptionalStorageMeta = `
storage:
  data0:
    type: block
  data1:
    type: block
    multiple:
      range: 0-
`

const twoRequiredStorageMeta = `
storage:
  data0:
    type: block
  data1:
    type: block
`

const twoOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
  data1:
    type: block
    multiple:
      range: 0-
`

const oneRequiredFilesystemStorageMeta = `
storage:
  data0:
    type: filesystem
`

const oneOptionalSharedStorageMeta = `
storage:
  data0:
    type: block
    shared: true
    multiple:
      range: 0-
`

const oneRequiredReadOnlyStorageMeta = `
storage:
  data0:
    type: block
    read-only: true
`

const oneRequiredLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
`

const oneMultipleLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
    multiple:
      range: 1-
`

func storageRange(min, max int) string {
	var minStr, maxStr string
	if min > 0 {
		minStr = fmt.Sprint(min)
	}
	if max > 0 {
		maxStr = fmt.Sprint(max)
	}
	return fmt.Sprintf(`
    multiple:
      range: %s-%s
`[1:], minStr, maxStr)
}

func (s *ApplicationSuite) setCharmFromMeta(c *gc.C, oldMeta, newMeta string) error {
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	return app.SetCharm(cfg)
}

func (s *ApplicationSuite) TestSetCharmOptionalUnusedStorageRemoved(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredOneOptionalStorageMeta,
		mysqlBaseMeta+oneRequiredStorageMeta,
	)
	c.Assert(err, jc.ErrorIsNil)
	// It's valid to remove optional storage so long
	// as it is not in use.
}

func (s *ApplicationSuite) TestSetCharmOptionalUsedStorageRemoved(c *gc.C) {
	oldMeta := mysqlBaseMeta + oneRequiredOneOptionalStorageMeta
	newMeta := mysqlBaseMeta + oneRequiredStorageMeta
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "test",
		Charm: oldCh,
		Storage: map[string]state.StorageConstraints{
			"data0": {Count: 1},
			"data1": {Count: 1},
		},
	})
	defer state.SetBeforeHooks(c, s.State, func() {
		// Adding a unit will cause the storage to be in-use.
		_, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": in-use storage "data1" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageRemoved(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": required storage "data0" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageAddedDefaultConstraints(c *gc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoRequiredStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the new required storage was added for the unit.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 2)
}

func (s *ApplicationSuite) TestSetCharmStorageAddedUserSpecifiedConstraints(c *gc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoOptionalStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:       newCh,
		CharmOrigin: defaultCharmOrigin(newCh.URL()),
		StorageConstraints: map[string]state.StorageConstraints{
			"data1": {Count: 3},
		},
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Check that new storage was added for the unit, based on the
	// constraints specified in SetCharmConfig.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 4)
}

func (s *ApplicationSuite) TestSetCharmOptionalStorageAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+twoOptionalStorageMeta,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinIncreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
	)
	// User must increase the storage constraints from 1 to 2.
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": validating storage constraints: charm "mysql" store "data0": 2 instances required, 1 specified`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 2),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 1),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from 2 to 1`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxUnboundedToBounded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, -1),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 999),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from \<unbounded\> to 999`)
}

func (s *ApplicationSuite) TestSetCharmStorageTypeChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" type changed from "block" to "filesystem"`)
}

func (s *ApplicationSuite) TestSetCharmStorageSharedChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneOptionalStorageMeta,
		mysqlBaseMeta+oneOptionalSharedStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" shared changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageReadOnlyChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredReadOnlyStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" read-only changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageLocationChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" location changed from "" to "/srv"`)
}

func (s *ApplicationSuite) TestSetCharmStorageWithLocationSingletonToMultipleAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
		mysqlBaseMeta+oneMultipleLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" with location changed from single to multiple`)
}

func (s *ApplicationSuite) assertApplicationRemovedWithItsBindings(c *gc.C, application *state.Application) {
	// Removing the application removes the bindings with it.
	err := application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsReturnsDefaultsWhenNotFound(c *gc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)
	state.RemoveEndpointBindingsForApplication(c, application)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
}

func (s *ApplicationSuite) assertApplicationHasOnlyDefaultEndpointBindings(c *gc.C, application *state.Application) {
	charm, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)

	knownEndpoints := set.NewStrings("")
	allBindings, err := state.DefaultEndpointBindingsForCharm(s.State, charm.Meta())
	c.Assert(err, jc.ErrorIsNil)
	for endpoint := range allBindings {
		knownEndpoints.Add(endpoint)
	}

	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings.Map(), gc.NotNil)

	for endpoint, space := range setBindings.Map() {
		c.Check(knownEndpoints.Contains(endpoint), jc.IsTrue)
		c.Check(space, gc.Equals, network.AlphaSpaceId, gc.Commentf("expected default space for endpoint %q, got %q", endpoint, space))
	}
}

func (s *ApplicationSuite) TestEndpointBindingsJustDefaults(c *gc.C) {
	// With unspecified bindings, all endpoints are explicitly bound to the
	// default space when saved in state.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsWithExplictOverrides(c *gc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	haSpace, err := s.State.AddSpace("ha", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	bindings := map[string]string{
		"server":  dbSpace.Id(),
		"cluster": haSpace.Id(),
	}
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, bindings)

	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings.Map(), jc.DeepEquals, map[string]string{
		"":        network.AlphaSpaceId,
		"server":  dbSpace.Id(),
		"client":  network.AlphaSpaceId,
		"cluster": haSpace.Id(),
	})

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmExtraBindingsUseDefaults(c *gc.C) {
	dbSpace, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 42)
	oldBindings := map[string]string{
		"server": dbSpace.Id(),
		"kludge": dbSpace.Id(),
		"client": dbSpace.Id(),
	}
	application := s.AddTestingApplicationWithBindings(c, "yoursql", oldCharm, oldBindings)
	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveOld := map[string]string{
		"":        network.AlphaSpaceId,
		"server":  dbSpace.Id(),
		"kludge":  dbSpace.Id(),
		"client":  dbSpace.Id(),
		"cluster": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), jc.DeepEquals, effectiveOld)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)

	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	setBindings, err = application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveNew := map[string]string{
		"": network.AlphaSpaceId,
		// These three should be preserved from oldCharm.
		"client":  dbSpace.Id(),
		"server":  dbSpace.Id(),
		"cluster": network.AlphaSpaceId,
		// "kludge" is missing in newMeta
		// All the remaining are new and use the empty default.
		"foo":  network.AlphaSpaceId,
		"baz":  network.AlphaSpaceId,
		"just": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), jc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmHandlesMissingBindingsAsDefaults(c *gc.C) {
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 69)
	app := s.AddTestingApplicationWithBindings(c, "theirsql", oldCharm, nil)
	state.RemoveEndpointBindingsForApplication(c, app)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 70)

	cfg := state.SetCharmConfig{
		Charm:       newCharm,
		CharmOrigin: defaultCharmOrigin(newCharm.URL()),
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	setBindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveNew := map[string]string{
		// The following two exist for both oldCharm and newCharm.
		"client":  network.AlphaSpaceId,
		"cluster": network.AlphaSpaceId,
		// "kludge" is missing in newMeta, "server" is new and gets the default.
		"server": network.AlphaSpaceId,
		// All the remaining are new and use the empty default.
		"foo":  network.AlphaSpaceId,
		"baz":  network.AlphaSpaceId,
		"just": network.AlphaSpaceId,
	}
	c.Assert(setBindings.Map(), jc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, app)
}

func (s *ApplicationSuite) setupApplicationWithUnitsForUpgradeCharmScenario(c *gc.C, numOfUnits int) (deployedV int, err error) {
	originalCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgreplication
`
	originalCharm := s.AddMetaCharm(c, "mysql", originalCharmMeta, 2)
	cfg := state.SetCharmConfig{Charm: originalCharm, CharmOrigin: defaultCharmOrigin(originalCharm.URL())}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:replication")
	deployedV = s.mysql.CharmModifiedVersion()

	for i := 0; i < numOfUnits; i++ {
		_, err = s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}

	// New mysql charm renames peer relation.
	updatedCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgpeer
`
	updatedCharm := s.AddMetaCharm(c, "mysql", updatedCharmMeta, 3)

	cfg = state.SetCharmConfig{Charm: updatedCharm, CharmOrigin: defaultCharmOrigin(updatedCharm.URL())}
	err = s.mysql.SetCharm(cfg)
	return
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithOneUnit(c *gc.C) {
	obtainedV, err := s.setupApplicationWithUnitsForUpgradeCharmScenario(c, 1)

	// ensure upgrade happened
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV+1, jc.IsTrue)
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithMoreThanOneUnit(c *gc.C) {
	obtainedV, err := s.setupApplicationWithUnitsForUpgradeCharmScenario(c, 2)

	// ensure upgrade happened
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV+1, jc.IsTrue)
}

func (s *ApplicationSuite) TestWatchCharmConfig(c *gc.C) {
	oldCharm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", oldCharm)
	// Add a unit so when we change the application's charm,
	// the old charm isn't removed (due to a reference).
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetCharmURL(oldCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	w, err := app.WatchCharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "superhero paparazzi"})
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", stringConfig, 123)
	err = app.SetCharm(state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application config for new charm; nothing detected.
	err = app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": "value"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

var updateApplicationConfigTests = []struct {
	about   string
	initial config.ConfigAttributes
	update  config.ConfigAttributes
	expect  config.ConfigAttributes
	err     string
}{{
	about:  "set string",
	update: config.ConfigAttributes{"outlook": "positive"},
	expect: config.ConfigAttributes{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: config.ConfigAttributes{"outlook": "positive"},
	update:  config.ConfigAttributes{"outlook": nil, "title": "sir"},
	expect:  config.ConfigAttributes{"title": "sir"},
}, {
	about:  "unset missing string",
	update: config.ConfigAttributes{"outlook": nil},
	expect: config.ConfigAttributes{},
}, {
	about:   `empty strings are valid`,
	initial: config.ConfigAttributes{"outlook": "positive"},
	update:  config.ConfigAttributes{"outlook": "", "title": ""},
	expect:  config.ConfigAttributes{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: config.ConfigAttributes{"title": "sir"},
	update:  config.ConfigAttributes{"username": "admin001"},
	expect:  config.ConfigAttributes{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: config.ConfigAttributes{"username": "admin001", "title": "sir"},
	update:  config.ConfigAttributes{"username": nil, "title": "My Title"},
	expect:  config.ConfigAttributes{"title": "My Title"},
}, {
	about:  "non-string type",
	update: config.ConfigAttributes{"skill-level": 303},
	expect: config.ConfigAttributes{"skill-level": 303},
}, {
	about:   "unset non-string type",
	initial: config.ConfigAttributes{"skill-level": 303},
	update:  config.ConfigAttributes{"skill-level": nil},
	expect:  config.ConfigAttributes{},
}}

func (s *ApplicationSuite) TestUpdateApplicationConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range updateApplicationConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateApplicationConfig(t.initial, nil, sampleApplicationConfigSchema(), nil)
			c.Assert(err, jc.ErrorIsNil)
		}
		updates := make(map[string]interface{})
		var resets []string
		for k, v := range t.update {
			if v == nil {
				resets = append(resets, k)
			} else {
				updates[k] = v
			}
		}
		err := app.UpdateApplicationConfig(updates, resets, sampleApplicationConfigSchema(), nil)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			cfg, err := app.ApplicationConfig()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cfg, gc.DeepEquals, t.expect)
		}
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestApplicationConfigNotFoundNoError(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", ch)

	// Delete all the settings. We should get a nil return, but no error.
	_, _ = s.State.MongoSession().DB("juju").C("settings").RemoveAll(nil)

	cfg, err := app.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestStatusInitial(c *gc.C) {
	appStatus, err := s.mysql.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(appStatus.Status, gc.Equals, status.Unset)
	c.Check(appStatus.Message, gc.Equals, "")
	c.Check(appStatus.Data, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestUnitStatusesNoUnits(c *gc.C) {
	statuses, err := s.mysql.UnitStatuses()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statuses, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestUnitStatusesWithUnits(c *gc.C) {
	u1, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u1.SetStatus(status.StatusInfo{
		Status: status.Maintenance,
	})
	c.Assert(err, jc.ErrorIsNil)

	// If Agent status is in error, we see that.
	u2, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u2.Agent().SetStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "foo",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = u2.SetStatus(status.StatusInfo{
		Status: status.Blocked,
	})
	c.Assert(err, jc.ErrorIsNil)

	statuses, err := s.mysql.UnitStatuses()
	c.Check(err, jc.ErrorIsNil)

	check := jc.NewMultiChecker()
	check.AddExpr(`_[_].Since`, jc.Ignore)
	check.AddExpr(`_[_].Data`, jc.Ignore)
	c.Assert(statuses, check, map[string]status.StatusInfo{
		"mysql/0": {
			Status: status.Maintenance,
		},
		"mysql/1": {
			Status:  status.Error,
			Message: "foo",
		},
	})
}

func sampleApplicationConfigSchema() environschema.Fields {
	schema := environschema.Fields{
		"title":       environschema.Attr{Type: environschema.Tstring},
		"outlook":     environschema.Attr{Type: environschema.Tstring},
		"username":    environschema.Attr{Type: environschema.Tstring},
		"skill-level": environschema.Attr{Type: environschema.Tint},
	}
	return schema
}

func (s *ApplicationSuite) TestUpdateApplicationConfigWithDyingApplication(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.UpdateApplicationConfig(config.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyApplicationRemovesConfig(c *gc.C) {
	err := s.mysql.UpdateApplicationConfig(config.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)
	appConfig := state.GetApplicationConfig(s.State, s.mysql)
	err = appConfig.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appConfig.Map(), gc.Not(gc.HasLen), 0)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
}

type CAASApplicationSuite struct {
	ConnSuite
	app    *state.Application
	caasSt *state.State
}

var _ = gc.Suite(&CAASApplicationSuite{})

func (s *CAASApplicationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.caasSt = s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { _ = s.caasSt.Close() })

	f := factory.NewFactory(s.caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	s.app = f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	// Consume the initial construction events from the watchers.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASApplicationSuite) TestUpdateCAASUnits(c *gc.C) {
	s.assertUpdateCAASUnits(c, true)
}

func (s *CAASApplicationSuite) TestUpdateCAASUnitsApplicationNotALive(c *gc.C) {
	s.assertUpdateCAASUnits(c, false)
}

func (s *CAASApplicationSuite) assertUpdateCAASUnits(c *gc.C, aliveApp bool) {
	existingUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("unit-uuid")})
	c.Assert(err, jc.ErrorIsNil)
	removedUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("removed-unit-uuid")})
	c.Assert(err, jc.ErrorIsNil)
	noContainerUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("never-cloud-container")})
	c.Assert(err, jc.ErrorIsNil)
	if !aliveApp {
		err := s.app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	var updateUnits state.UpdateUnitsOperation
	updateUnits.Deletes = []*state.DestroyUnitOperation{removedUnit.DestroyOperation()}
	updateUnits.Adds = []*state.AddUnitOperation{
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("new-unit-uuid"),
			Address:    strPtr("192.168.1.1"),
			Ports:      &[]string{"80"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new container running",
			},
		}),
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("add-never-cloud-container"),
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			// Status history should not show this as active.
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
	}
	updateUnits.Updates = []*state.UpdateUnitOperation{
		noContainerUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("never-cloud-container"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("unit-uuid"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing container running",
			},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	if !aliveApp {
		c.Assert(err, jc.Satisfies, state.IsNotAlive)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	units, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 4)

	unitsById := make(map[string]*state.Unit)
	containerInfoById := make(map[string]state.CloudContainer)
	for _, u := range units {
		c.Assert(u.ShouldBeAssigned(), jc.IsFalse)
		containerInfo, err := u.ContainerInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(containerInfo.Unit(), gc.Equals, u.Name())
		c.Assert(containerInfo.ProviderId(), gc.Not(gc.Equals), "")
		unitsById[containerInfo.ProviderId()] = u
		containerInfoById[containerInfo.ProviderId()] = containerInfo
	}
	u, ok := unitsById["unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	info, ok := containerInfoById["unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u.Name(), gc.Equals, existingUnit.Name())
	c.Check(info.Address(), gc.NotNil)
	c.Check(*info.Address(), gc.DeepEquals,
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeMachineLocal)))
	c.Check(info.Ports(), jc.DeepEquals, []string{"443"})
	statusInfo, err := u.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "existing running")
	history, err := u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, gc.Equals, status.Running)
		c.Assert(history[1].Status, gc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, gc.Equals, status.Running)
		c.Assert(history[0].Status, gc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}
	statusInfo, err = u.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, gc.Equals, "installing agent")
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "existing container running")
	unitHistory, err := u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if unitHistory[0].Status == status.Running {
		c.Assert(unitHistory[0].Status, gc.Equals, status.Running)
		c.Assert(unitHistory[0].Message, gc.Equals, "existing container running")
		c.Assert(unitHistory[1].Status, gc.Equals, status.Waiting)
	} else {
		c.Assert(unitHistory[1].Status, gc.Equals, status.Running)
		c.Assert(unitHistory[1].Message, gc.Equals, "existing container running")
		c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
		c.Assert(unitHistory[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}

	u, ok = unitsById["never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	info, ok = containerInfoById["never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, gc.Equals, status.MessageInstallingAgent)

	u, ok = unitsById["add-never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	info, ok = containerInfoById["add-never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, gc.Equals, status.MessageInstallingAgent)

	u, ok = unitsById["new-unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	info, ok = containerInfoById["new-unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(u.Name(), gc.Equals, "gitlab/3")
	c.Check(info.Address(), gc.NotNil)
	c.Check(*info.Address(), gc.DeepEquals,
		network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal)))
	c.Assert(info.Ports(), jc.DeepEquals, []string{"80"})

	addr, err := u.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal)))

	statusInfo, err = u.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, gc.Equals, status.MessageInstallingAgent)
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "new container running")
	statusInfo, err = u.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "new running")
	history, err = u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, gc.Equals, status.Running)
		c.Assert(history[1].Status, gc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, gc.Equals, status.Running)
		c.Assert(history[0].Status, gc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}
	// container status history must have overridden the unit status.
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if unitHistory[0].Status == status.Running {
		c.Assert(unitHistory[0].Status, gc.Equals, status.Running)
		c.Assert(unitHistory[0].Message, gc.Equals, "new container running")
		c.Assert(unitHistory[1].Status, gc.Equals, status.Waiting)
	} else {
		c.Assert(unitHistory[1].Status, gc.Equals, status.Running)
		c.Assert(unitHistory[1].Message, gc.Equals, "new container running")
		c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
		c.Assert(unitHistory[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}

	// check cloud container status history is stored.
	containerStatusHistory, err := state.GetCloudContainerStatusHistory(s.caasSt, u.Name(), status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerStatusHistory, gc.HasLen, 1)
	c.Assert(containerStatusHistory[0].Status, gc.Equals, status.Running)
	c.Assert(containerStatusHistory[0].Message, gc.Equals, "new container running")

	err = removedUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestAddUnitWithProviderId(c *gc.C) {
	u, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, jc.ErrorIsNil)
	info, err := u.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, u.Name())
	c.Assert(info.ProviderId(), gc.Equals, "provider-id")
}

func (s *CAASApplicationSuite) TestServiceInfo(c *gc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("id", addrs)
		c.Assert(err, jc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, jc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.ProviderId(), gc.Equals, "id")
		c.Assert(info.Addresses(), jc.DeepEquals, addrs)
	}
}

func (s *CAASApplicationSuite) TestServiceInfoEmptyProviderId(c *gc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("", addrs)
		c.Assert(err, jc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, jc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.ProviderId(), gc.Equals, "")
		c.Assert(info.Addresses(), jc.DeepEquals, addrs)
	}
}

func (s *CAASApplicationSuite) TestRemoveApplicationDeletesServiceInfo(c *gc.C) {
	addrs := network.NewSpaceAddresses("10.0.0.1")

	err := s.app.UpdateCloudService("id", addrs)
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	// Until cleanups run, no removal.
	si, err := s.app.ServiceInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si, gc.NotNil)
	assertCleanupCount(c, s.caasSt, 2)
	_, err = s.app.ServiceInfo()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestInvalidScale(c *gc.C) {
	err := s.app.SetScale(-1, 0, true)
	c.Assert(err, gc.ErrorMatches, "application scale -1 not valid")

	// set scale without force for caas workers - a new Generation is required.
	err = s.app.SetScale(3, 0, false)
	c.Assert(err, jc.Satisfies, errors.IsForbidden)
}

func (s *CAASApplicationSuite) TestSetScale(c *gc.C) {
	// set scale with force for CLI - DesiredScaleProtected set to true.
	err := s.app.SetScale(5, 0, true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.GetScale(), gc.Equals, 5)
	svcInfo, err := s.app.ServiceInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svcInfo.DesiredScaleProtected(), jc.IsTrue)

	// set scale without force for caas workers - a new Generation is required.
	err = s.app.SetScale(5, 1, false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.GetScale(), gc.Equals, 5)
	svcInfo, err = s.app.ServiceInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svcInfo.DesiredScaleProtected(), jc.IsFalse)
	c.Assert(svcInfo.Generation(), jc.DeepEquals, int64(1))
}

func (s *CAASApplicationSuite) TestInvalidChangeScale(c *gc.C) {
	newScale, err := s.app.ChangeScale(-1)
	c.Assert(err, gc.ErrorMatches, "cannot remove more units than currently exist not valid")
	c.Assert(newScale, gc.Equals, 0)
}

func (s *CAASApplicationSuite) TestChangeScale(c *gc.C) {
	newScale, err := s.app.ChangeScale(5)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newScale, gc.Equals, 5)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.GetScale(), gc.Equals, 5)

	newScale, err = s.app.ChangeScale(-4)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newScale, gc.Equals, 1)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.GetScale(), gc.Equals, 1)
}

func (s *CAASApplicationSuite) TestWatchScale(c *gc.C) {
	// Empty initial event.
	w := s.app.WatchScale()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.app.SetScale(5, 0, true)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set to same value, no change.
	err = s.app.SetScale(5, 0, true)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.SetScale(6, 0, true)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// An unrelated update, no change.
	err = s.app.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *CAASApplicationSuite) TestWatchCloudService(c *gc.C) {
	cloudSvc, err := s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id: s.app.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w := cloudSvc.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	_, err = s.State.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         s.app.Name(),
		ProviderId: "123",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove service by removing app, start new watch, check single event.
	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = cloudSvc.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *CAASApplicationSuite) TestRewriteStatusHistory(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	history, err := app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 1)
	c.Assert(history[0].Status, gc.Equals, status.Unset)
	c.Assert(history[0].Message, gc.Equals, "")

	// Must overwrite the history
	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Allocating,
		Message: "operator message",
	})
	c.Assert(err, jc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Status, gc.Equals, status.Allocating)
	c.Assert(history[0].Message, gc.Equals, "operator message")
	c.Assert(history[1].Status, gc.Equals, status.Unset)
	c.Assert(history[1].Message, gc.Equals, "")

	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Running,
		Message: "operator running",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = app.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "app active",
	})
	c.Assert(err, jc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Log(history)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Status, gc.Equals, status.Active)
	c.Assert(history[0].Message, gc.Equals, "app active")
	c.Assert(history[1].Status, gc.Equals, status.Allocating)
	c.Assert(history[1].Message, gc.Equals, "operator message")
	c.Assert(history[2].Status, gc.Equals, status.Unset)
	c.Assert(history[2].Message, gc.Equals, "")
}

func (s *CAASApplicationSuite) TestClearResources(c *gc.C) {
	c.Assert(state.GetApplicationHasResources(s.app), jc.IsTrue)
	err := s.app.ClearResources()
	c.Assert(err, gc.ErrorMatches, `application "gitlab" is alive`)
	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)

	// ClearResources should be idempotent.
	for i := 0; i < 2; i++ {
		err := s.app.ClearResources()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(state.GetApplicationHasResources(s.app), jc.IsFalse)
	}
	// Resetting the app's HasResources the first time schedules a cleanup.
	assertCleanupCount(c, s.caasSt, 2)
}

func (s *CAASApplicationSuite) TestDestroySimple(c *gc.C) {
	err := s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	c.Assert(s.app.Life(), gc.Equals, state.Dead)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.GetApplicationHasResources(s.app), jc.IsTrue)
}

func (s *CAASApplicationSuite) TestForceDestroyQueuesForceCleanup(c *gc.C) {
	op := s.app.DestroyOperation()
	op.Force = true
	err := s.caasSt.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)

	// Cleanup queued but won't run until scheduled.
	assertNeedsCleanup(c, s.caasSt)
	s.Clock.Advance(2 * time.Minute)
	assertCleanupRuns(c, s.caasSt)

	err = s.app.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyStillHasUnits(c *gc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.Life(), gc.Equals, state.Dying)

	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)

	c.Assert(unit.Remove(), jc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyOnceHadUnits(c *gc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.Life(), gc.Equals, state.Dead)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyStaleNonZeroUnitCount(c *gc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.Life(), gc.Equals, state.Dead)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyStaleZeroUnitCount(c *gc.C) {
	unit, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.Life(), gc.Equals, state.Dying)
	assertLife(c, s.app, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)

	c.Assert(unit.Remove(), jc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	c.Assert(err, jc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)
}

func (s *CAASApplicationSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	ch := state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "mysql")
	mysql := state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "mysql", ch)
	eps, err := s.caasSt.InferEndpoints("gitlab", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a application with no units in relation scope; check application and
	// unit removed.
	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, mysql, state.Dead)

	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *CAASApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *CAASApplicationSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	ch := state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "mysql")
	mysql := state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "mysql", ch)
	eps, err := s.caasSt.InferEndpoints("gitlab", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	ch = state.AddTestingCharmForSeries(c, s.caasSt, "kubernetes", "proxy")
	state.AddTestingApplicationForBase(c, s.caasSt, state.UbuntuBase("20.04"), "proxy", ch)
	eps, err = s.caasSt.InferEndpoints("proxy", "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.caasSt.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	c.Assert(s.app.Destroy(), jc.ErrorIsNil)
	assertLife(c, rel0, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are are both removed.
	c.Assert(ru.LeaveScope(), jc.ErrorIsNil)
	assertCleanupCount(c, s.caasSt, 1)
	// App not removed since cluster resources not cleaned up yet.
	assertLife(c, s.app, state.Dead)

	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestDestroyQueuesUnitCleanup(c *gc.C) {
	// Add 5 units; block quick-remove of gitlab/1 and gitlab/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			unitState := state.NewUnitState()
			unitState.SetUniterState("idle")
			err := unit.SetState(unitState, state.UnitStateSizeLimits{})
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	assertDoesNotNeedCleanup(c, s.caasSt)

	// Destroy gitlab, and check units are not touched.
	err := s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.app, state.Dying)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	dirty, err := s.caasSt.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
	assertCleanupCount(c, s.caasSt, 2)

	for i, unit := range units {
		if i%2 != 0 {
			assertLife(c, unit, state.Dying)
		} else {
			assertRemoved(c, unit)
		}
	}

	// App dying until units are gone.
	assertLife(c, s.app, state.Dying)
}

func (s *CAASApplicationSuite) TestGetUnitAttachmentInfosWithoutAttachStorage(c *gc.C) {
	app := s.setupApplicationWithAttachStorage(c, 2, []names.StorageTag{})
	infos, err := app.GetUnitAttachmentInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infos, gc.HasLen, 0)
}

func (s *CAASApplicationSuite) TestGetUnitAttachmentInfosWithAttachStorage(c *gc.C) {
	app := s.setupApplicationWithAttachStorage(c, 1, []names.StorageTag{names.NewStorageTag("database/0")})
	infos, err := app.GetUnitAttachmentInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infos, gc.DeepEquals, []state.UnitAttachmentInfo{{Unit: "cockroachdb/0", VolumeId: "pv-database-0", StorageId: "database/0"}})
}

func (s *CAASApplicationSuite) setupApplicationWithAttachStorage(c *gc.C, unitNum int, attachStorage []names.StorageTag) *state.Application {
	registry := &storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummy.StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
		},
	}

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName: "caascloud",
	})
	s.AddCleanup(func(_ *gc.C) { _ = st.Close() })

	pm := poolmanager.New(state.NewStateSettings(st), registry)
	_, err := pm.Create("kubernetes", "kubernetes", map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i <= unitNum; i++ {
		fsInfo := state.FilesystemInfo{
			Size: 100,
			Pool: "kubernetes",
		}
		volumeInfo := state.VolumeInfo{
			VolumeId:   fmt.Sprintf("pv-database-%d", i),
			Size:       100,
			Pool:       "kubernetes",
			Persistent: true,
		}
		storageTag, err := sb.AddExistingFilesystem(fsInfo, &volumeInfo, "database")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(storageTag.Id(), gc.Equals, fmt.Sprintf("database/%d", i))
	}

	ch := state.AddTestingCharmForSeries(c, st, "quantal", "cockroachdb")
	cockroachdb := state.AddTestingApplicationWithAttachStorage(c, st, "cockroachdb", ch, unitNum,
		map[string]state.StorageConstraints{
			"database": {
				Pool:  "kubernetes",
				Size:  100,
				Count: 0,
			},
		},
		attachStorage,
	)
	units, err := cockroachdb.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, unitNum)
	return cockroachdb
}

func (s *ApplicationSuite) TestSetOperatorStatusNonCAAS(c *gc.C) {
	_, err := state.ApplicationOperatorStatus(s.State, s.mysql.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetOperatorStatus(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "broken",
		Since:   &now,
	}
	err := app.SetOperatorStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	appStatus, err := state.ApplicationOperatorStatus(st, app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.DeepEquals, status.Error)
	c.Assert(appStatus.Message, gc.DeepEquals, "broken")
}

func (s *ApplicationSuite) TestCharmLegacyOnlySupportsOneSeries(c *gc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "precise", "mysql")
	app := s.AddTestingApplication(c, "legacy-charm", ch)
	err := app.VerifySupportedBase(state.UbuntuBase("12.10"))
	c.Assert(err, jc.ErrorIsNil)
	err = app.VerifySupportedBase(state.UbuntuBase("16.04"))
	c.Assert(err, gc.ErrorMatches, "base \"ubuntu@16.04\" not supported by charm, the charm supported bases are: ubuntu@12.10")
}

func (s *ApplicationSuite) TestCharmLegacyNoOSInvalid(c *gc.C) {
	ch := state.AddTestingCharmForSeries(c, s.State, "precise", "sample-fail-no-os")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "sample-fail-no-os",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "22.04/stable",
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, `.*charm does not define any bases`)
}

func (s *ApplicationSuite) TestDeployedMachines(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "riak"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	machines, err := app.DeployedMachines()

	c.Assert(err, jc.ErrorIsNil)
	var ids []string
	for _, m := range machines {
		ids = append(ids, m.Id())
	}
	c.Assert(ids, jc.SameContents, []string{"0"})
}

func (s *ApplicationSuite) TestDeployedMachinesNotAssignedUnit(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "riak"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignedMachineId()
	c.Assert(err, jc.Satisfies, errors.IsNotAssigned)

	machines, err := app.DeployedMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestCAASSidecarCharm(c *gc.C) {
	st, app := s.addCAASSidecarApplication(c)
	defer st.Close()
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	sidecar, err := unit.IsSidecar()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sidecar, jc.IsTrue)
}

func (s *ApplicationSuite) addCAASSidecarApplication(c *gc.C) (*state.State, *state.Application) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	f := factory.NewFactory(st, s.StatePool)

	charmDef := `
name: cockroachdb
description: foo
summary: foo
containers:
  redis:
    resource: redis-container-resource
resources:
  redis-container-resource:
    name: redis-container
    type: oci-image
provides:
  data-port:
    interface: data
    scope: container
`
	ch := state.AddCustomCharmWithManifest(c, st, "cockroach", "metadata.yaml", charmDef, "focal", 1)
	return st, f.MakeApplication(c, &factory.ApplicationParams{Name: "cockroachdb", Charm: ch})
}

func (s *ApplicationSuite) TestCAASNonSidecarCharm(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)

	charmDef := `
name: mysql
description: foo
summary: foo
series:
  - kubernetes
deployment:
  mode: workload
`
	ch := state.AddCustomCharmForSeries(c, st, "mysql", "metadata.yaml", charmDef, "kubernetes", 1)
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch})

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	sidecar, err := unit.IsSidecar()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sidecar, jc.IsFalse)
}

func (s *ApplicationSuite) TestWatchApplicationsWithPendingCharms(c *gc.C) {
	w := s.State.WatchApplicationsWithPendingCharms()
	defer func() { _ = w.Stop() }()

	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange() // consume initial change set.

	// Add a pending charm with an origin and associate it with the
	// application. This should trigger a change.
	dummy2 := s.dummyCharm(c, "ch:dummy-1")
	dummy2.SHA256 = ""      // indicates that we don't have the data in the blobstore yet.
	dummy2.StoragePath = "" // indicates that we don't have the data in the blobstore yet.
	ch2, err := s.State.AddCharmMetadata(dummy2)
	c.Assert(err, jc.ErrorIsNil)
	twoOrigin := defaultCharmOrigin(ch2.URL())
	twoOrigin.Platform.OS = "ubuntu"
	twoOrigin.Platform.Channel = "22.04/stable"
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       ch2,
		CharmOrigin: twoOrigin,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(s.mysql.Name())

	// "Upload" a charm and check that we don't get a notification for it.
	dummy3 := s.dummyCharm(c, "ch:dummy-2")
	ch3, err := s.State.AddCharm(dummy3)
	c.Assert(err, jc.ErrorIsNil)
	threeOrigin := defaultCharmOrigin(ch3.URL())
	threeOrigin.Platform.OS = "ubuntu"
	threeOrigin.Platform.Channel = "22.04/stable"
	threeOrigin.ID = "charm-hub-id"
	threeOrigin.Hash = "charm-hub-hash"
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:       ch3,
		CharmOrigin: threeOrigin,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	origin := &state.CharmOrigin{
		Source: "charm-hub",
		Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		},
	}
	// Simulate a bundle deploying multiple applications from a single
	// charm. The watcher needs to notify on the secondary applications.
	appSameCharm, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        "mysql-testing",
		Charm:       ch3,
		CharmOrigin: origin,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(appSameCharm.Name())
	origin.ID = "charm-hub-id"
	origin.Hash = "charm-hub-hash"
	_ = appSameCharm.SetCharm(state.SetCharmConfig{
		Charm:       ch3,
		CharmOrigin: origin,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationSuite) dummyCharm(c *gc.C, curlOverride string) state.CharmInfo {
	info := state.CharmInfo{
		Charm:       testcharms.Repo.CharmDir("dummy"),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
		Version:     "dummy-146-g725cfd3-dirty",
	}
	if curlOverride != "" {
		info.ID = curlOverride
	} else {
		info.ID = fmt.Sprintf("local:quantal/%s-%d", info.Charm.Meta().Name, info.Charm.Revision())
	}
	info.Charm.Meta().Series = []string{"quantal", "jammy"}
	return info
}

func (s *ApplicationSuite) TestWatch(c *gc.C) {
	w := s.mysql.WatchConfigSettingsHash()
	defer testing.AssertStop(c, w)

	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("1e11259677ef769e0ec4076b873c76dcc3a54be7bc651b081d0f0e2b87077717")

	schema := environschema.Fields{
		"username":    environschema.Attr{Type: environschema.Tstring},
		"alive":       environschema.Attr{Type: environschema.Tbool},
		"skill-level": environschema.Attr{Type: environschema.Tint},
		"options":     environschema.Attr{Type: environschema.Tattrs},
	}

	err := s.mysql.UpdateApplicationConfig(config.ConfigAttributes{
		"username":    "abbas",
		"alive":       true,
		"skill-level": 23,
		"options": map[string]string{
			"fortuna": "crescis",
			"luna":    "velut",
			"status":  "malus",
		},
	}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange("e1471e8a7299da0ac2150445ffc6d08d9d801194037d88416c54b01899b8a9b2")
}

func (s *ApplicationSuite) TestProvisioningState(c *gc.C) {
	ps := s.mysql.ProvisioningState()
	c.Assert(ps, gc.IsNil)

	err := s.mysql.SetProvisioningState(state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Assert(errors.Is(err, stateerrors.ProvisioningStateInconsistent), jc.IsTrue)

	err = s.mysql.SetScale(10, 0, true)
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.SetProvisioningState(state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Assert(err, jc.ErrorIsNil)

	ps = s.mysql.ProvisioningState()
	c.Assert(ps, jc.DeepEquals, &state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
}

func (s *CAASApplicationSuite) TestUpsertCAASUnit(c *gc.C) {
	registry := &storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"kubernetes": &dummy.StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
		},
	}

	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName: "caascloud",
	})
	s.AddCleanup(func(_ *gc.C) { _ = st.Close() })

	pm := poolmanager.New(state.NewStateSettings(st), registry)
	_, err := pm.Create("kubernetes", "kubernetes", map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)

	fsInfo := state.FilesystemInfo{
		Size: 100,
		Pool: "kubernetes",
	}
	volumeInfo := state.VolumeInfo{
		VolumeId:   "pv-database-0",
		Size:       100,
		Pool:       "kubernetes",
		Persistent: true,
	}
	storageTag, err := sb.AddExistingFilesystem(fsInfo, &volumeInfo, "database")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageTag.Id(), gc.Equals, "database/0")

	ch := state.AddTestingCharmForSeries(c, st, "quantal", "cockroachdb")
	cockroachdb := state.AddTestingApplicationWithStorage(c, st, "cockroachdb", ch, map[string]state.StorageConstraints{
		"database": {
			Pool:  "kubernetes",
			Size:  100,
			Count: 0,
		},
	})

	unitName := "cockroachdb/0"
	providerId := "cockroachdb-0"
	address := "1.2.3.4"
	ports := []string{"80", "443"}

	// output of utils.AgentPasswordHash("juju")
	passwordHash := "v+jK3ht5NEdKeoQBfyxmlYe0"

	p := state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			UnitName:     &unitName,
			ProviderId:   &providerId,
			Address:      &address,
			Ports:        &ports,
			PasswordHash: &passwordHash,
		},
		OrderedScale:              true,
		OrderedId:                 0,
		ObservedAttachedVolumeIDs: []string{"pv-database-0"},
	}
	unit, err := cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, gc.ErrorMatches, `unrequired unit cockroachdb/0 is not assigned`)
	c.Assert(unit, gc.IsNil)

	err = cockroachdb.SetScale(1, 0, true)
	c.Assert(err, jc.ErrorIsNil)

	unit, err = cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit, gc.NotNil)
	c.Assert(unit.UnitTag().Id(), gc.Equals, "cockroachdb/0")
	c.Assert(unit.Life(), gc.Equals, state.Alive)
	containerInfo, err := unit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerInfo.ProviderId(), gc.Equals, "cockroachdb-0")
	c.Assert(containerInfo.Ports(), jc.SameContents, []string{"80", "443"})
	c.Assert(containerInfo.Address().Value, gc.Equals, "1.2.3.4")

	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = sb.DetachStorage(storageTag, unit.UnitTag(), false, 0)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.DetachFilesystem(unit.UnitTag(), names.NewFilesystemTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveFilesystemAttachment(unit.UnitTag(), names.NewFilesystemTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.DetachVolume(unit.Tag(), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveVolumeAttachment(unit.Tag(), names.NewVolumeTag("0"), false)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	unit2, err := cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, gc.ErrorMatches, `dead unit "cockroachdb/0" already exists`)
	c.Assert(unit2, gc.IsNil)

	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = st.Cleanup(fakeSecretDeleter)
	c.Assert(err, jc.ErrorIsNil)

	unit, err = cockroachdb.UpsertCAASUnit(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit, gc.NotNil)
	c.Assert(unit.UnitTag().Id(), gc.Equals, "cockroachdb/0")
	c.Assert(unit.Life(), gc.Equals, state.Alive)
	containerInfo, err = unit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerInfo.ProviderId(), gc.Equals, "cockroachdb-0")
	c.Assert(containerInfo.Ports(), jc.SameContents, []string{"80", "443"})
	c.Assert(containerInfo.Address().Value, gc.Equals, "1.2.3.4")
}

func intPtr(val int) *int {
	return &val
}

func defaultCharmOrigin(curlStr string) *state.CharmOrigin {
	// Use ParseURL here in test until either the charm and/or application
	// can easily provide the same data.
	curl, _ := charm.ParseURL(curlStr)
	var source string
	var channel *state.Channel
	if charm.CharmHub.Matches(curl.Schema) {
		source = corecharm.CharmHub.String()
		channel = &state.Channel{
			Risk: "stable",
		}
	} else if charm.Local.Matches(curl.Schema) {
		source = corecharm.Local.String()
	}

	base, _ := corebase.GetBaseFromSeries(curl.Series)

	platform := &state.Platform{
		Architecture: corearch.DefaultArchitecture,
		OS:           base.OS,
		Channel:      base.Channel.String(),
	}

	return &state.CharmOrigin{
		Source:   source,
		Type:     "charm",
		Revision: intPtr(curl.Revision),
		Channel:  channel,
		Platform: platform,
	}
}
