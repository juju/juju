// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/juju/testcharms"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type charmhubSuite struct {
	jujutesting.JujuConnSuite

	updater *charmrevisionupdater.CharmRevisionUpdaterAPI
	charms  map[string]*state.Charm
}

var _ = gc.Suite(&charmhubSuite{})

func (s *charmhubSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *charmhubSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	authorizer := testing.FakeAuthorizer{
		Controller: true,
		Tag:        names.NewMachineTag("99"),
	}
	var err error
	s.updater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, resources, authorizer)
	c.Assert(err, jc.ErrorIsNil)

	// Patch the charmhub initializer function
	s.PatchValue(&charmrevisionupdater.NewCharmhubClient, func(st *state.State) (charmrevisionupdater.CharmhubRefreshClient, error) {
		charms := map[string]hubCharm{
			//"000": {
			//	id: "000",
			//	name: "postgresql",
			//	version: "10",
			//	resources: []transport.ResourceRevision{
			//		{Name: "wal-e", Revision: 2, Type: "file"},
			//	},
			//},
			"001": {
				id:      "001",
				name:    "mysql",
				version: "23",
			},
		}
		return &mockHub{charms: charms}, nil
	})

	s.charms = make(map[string]*state.Charm)
}

type hubCharm struct {
	id        string
	name      string
	version   string
	resources []transport.ResourceRevision
}

type mockHub struct {
	charms map[string]hubCharm
}

func (h *mockHub) Refresh(_ context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
	request, err := config.Build()
	if err != nil {
		return nil, err
	}
	responses := make([]transport.RefreshResponse, len(request.Context))
	for i, context := range request.Context {
		action := request.Actions[i]
		if action.Action != "refresh" {
			return nil, errors.Errorf("unexpected action %q", action.Action)
		}
		if *action.ID != context.ID {
			return nil, errors.Errorf("action ID %q doesn't match context ID %q", *action.ID, context.ID)
		}
		charm, ok := h.charms[context.ID]
		if !ok {
			return nil, errors.Errorf("charm ID %q not found", context.ID)
		}
		response := transport.RefreshResponse{
			Entity: transport.RefreshEntity{
				CreatedAt: time.Now(),
				ID:        context.ID,
				Name:      charm.name,
				Resources: charm.resources,
				Version:   charm.version,
			},
			EffectiveChannel: context.TrackingChannel,
			ID:               context.ID,
			InstanceKey:      context.InstanceKey,
			Name:             charm.name,
			Result:           "refresh",
		}
		responses[i] = response
	}
	return responses, nil
}

func (s *charmhubSuite) addMachine(c *gc.C, machineId string, job state.MachineJob) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, machineId)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := jujutesting.AssertStartInstanceWithConstraints(c, s.Environ, s.ProviderCallContext, controllerCfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmhubSuite) addCharmWithRevision(c *gc.C, charmName string, rev int) {
	ch := testcharms.Repo.CharmDir(charmName)
	name := ch.Meta().Name
	curl := &charm.URL{
		Schema:   "ch",
		Name:     charmName,
		Revision: rev,
		Series:   "quantal",
	}
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
}

func (s *charmhubSuite) addApplication(c *gc.C, charmName, serviceName string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: serviceName, Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmhubSuite) addUnit(c *gc.C, serviceName, machineId string) {
	svc, err := s.State.Application(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := svc.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmhubSuite) TestUpdateRevisions(c *gc.C) {
	s.addMachine(c, "0", state.JobManageModel)

	s.addMachine(c, "1", state.JobHostUnits)
	s.addMachine(c, "2", state.JobHostUnits)
	s.addMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.addCharmWithRevision(c, "mysql", 22)
	s.addApplication(c, "mysql", "mysql")
	s.addUnit(c, "mysql", "1")

	curl := charm.MustParseURL("ch:mysql")
	_, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err := s.updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	curl = charm.MustParseURL("ch:mysql")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.String(), gc.Equals, "ch:mysql-23")
}
