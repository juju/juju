// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type baseSuite struct {
	testing.JujuConnSuite
	commontesting.BlockHelper
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

var _ = gc.Suite(&baseSuite{})

// apiAuthenticator represents a simple authenticator object with only the
// SetPassword and Tag methods.  This will fit types from both the state
// and api packages, as those in the api package do not have PasswordValid().
type apiAuthenticator interface {
	state.Entity
	SetPassword(string) error
}

func setDefaultPassword(c *gc.C, e apiAuthenticator) {
	err := e.SetPassword(defaultPassword(e))
	c.Assert(err, jc.ErrorIsNil)
}

func defaultPassword(e apiAuthenticator) string {
	return e.Tag().String() + " password-1234567890"
}

type setStatuser interface {
	SetStatus(status.StatusInfo) error
}

func setDefaultStatus(c *gc.C, entity setStatuser) {
	now := time.Now()
	s := status.StatusInfo{
		Status:  status.Started,
		Message: "",
		Since:   &now,
	}
	err := entity.SetStatus(s)
	c.Assert(err, jc.ErrorIsNil)
}

// openAs connects to the API state as the given entity
// with the default password for that entity.
func (s *baseSuite) openAs(c *gc.C, tag names.Tag) api.Connection {
	info := s.APIInfo(c)
	info.Tag = tag
	// Must match defaultPassword()
	info.Password = fmt.Sprintf("%s password-1234567890", tag)
	// Set this always, so that the login attempts as a machine will
	// not fail with ErrNotProvisioned; it's not used otherwise.
	info.Nonce = "fake_nonce"
	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	return st
}

// scenarioStatus describes the expected state
// of the juju model set up by setUpScenario.
var scenarioStatus = &params.FullStatus{
	Model: params.ModelStatusInfo{
		Name:        "controller",
		Type:        "iaas",
		CloudTag:    "cloud-dummy",
		CloudRegion: "dummy-region",
		Version:     "1.2.3",
		ModelStatus: params.DetailedStatus{
			Status: "available",
		},
		SLA: "unsupported",
	},
	Machines: map[string]params.MachineStatus{
		"0": {
			Id:         "0",
			InstanceId: instance.Id("i-machine-0"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.Pending.String(),
			},
			ModificationStatus: params.DetailedStatus{
				Status: status.Idle.String(),
			},
			Series:     "quantal",
			Containers: map[string]params.MachineStatus{},
			Jobs:       []model.MachineJob{model.JobManageModel},
			HasVote:    false,
			WantsVote:  true,
		},
		"1": {
			Id:         "1",
			InstanceId: instance.Id("i-machine-1"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.Pending.String(),
			},
			ModificationStatus: params.DetailedStatus{
				Status: status.Idle.String(),
			},
			Series:     "quantal",
			Containers: map[string]params.MachineStatus{},
			Jobs:       []model.MachineJob{model.JobHostUnits},
			HasVote:    false,
			WantsVote:  false,
		},
		"2": {
			Id:         "2",
			InstanceId: instance.Id("i-machine-2"),
			AgentStatus: params.DetailedStatus{
				Status: "started",
			},
			InstanceStatus: params.DetailedStatus{
				Status: status.Pending.String(),
			},
			ModificationStatus: params.DetailedStatus{
				Status: status.Idle.String(),
			},
			Series:      "quantal",
			Constraints: "mem=1024M",
			Containers:  map[string]params.MachineStatus{},
			Jobs:        []model.MachineJob{model.JobHostUnits},
			HasVote:     false,
			WantsVote:   false,
		},
	},
	RemoteApplications: map[string]params.RemoteApplicationStatus{
		"remote-db2": {
			OfferURL:  "admin/prod.db2",
			OfferName: "remote-db2",
			Endpoints: []params.RemoteEndpoint{{
				Name:      "database",
				Interface: "db2",
				Role:      "provider",
			}},
			Relations: map[string][]string{},
			Status: params.DetailedStatus{
				Status: status.Unknown.String(),
			},
		},
	},
	Offers: map[string]params.ApplicationOfferStatus{
		"hosted-mysql": {
			CharmURL:        "local:quantal/mysql-1",
			ApplicationName: "mysql",
			OfferName:       "hosted-mysql",
			Endpoints: map[string]params.RemoteEndpoint{
				"database": {
					Name:      "server",
					Interface: "mysql",
					Role:      "provider",
				}},
			ActiveConnectedCount: 0,
			TotalConnectedCount:  1,
		},
	},
	Applications: map[string]params.ApplicationStatus{
		"logging": {
			Charm:  "local:quantal/logging-1",
			Series: "quantal",
			Relations: map[string][]string{
				"logging-directory": {"wordpress"},
			},
			SubordinateTo: []string{"wordpress"},
			Status: params.DetailedStatus{
				Status: "waiting",
				Info:   "waiting for machine",
			},
			EndpointBindings: map[string]string{
				"":                  network.AlphaSpaceName,
				"info":              network.AlphaSpaceName,
				"logging-client":    network.AlphaSpaceName,
				"logging-directory": network.AlphaSpaceName,
			},
		},
		"mysql": {
			Charm:         "local:quantal/mysql-1",
			Series:        "quantal",
			Relations:     map[string][]string{},
			SubordinateTo: []string{},
			Units:         map[string]params.UnitStatus{},
			Status: params.DetailedStatus{
				Status: "waiting",
				Info:   "waiting for machine",
			},
			EndpointBindings: map[string]string{
				"":               network.AlphaSpaceName,
				"server":         network.AlphaSpaceName,
				"server-admin":   network.AlphaSpaceName,
				"metrics-client": network.AlphaSpaceName,
			},
		},
		"wordpress": {
			Charm:  "local:quantal/wordpress-3",
			Series: "quantal",
			Relations: map[string][]string{
				"logging-dir": {"logging"},
			},
			SubordinateTo: []string{},
			Status: params.DetailedStatus{
				Status: "error",
				Info:   "blam",
				Data:   map[string]interface{}{"remote-unit": "logging/0", "foo": "bar", "relation-id": "0"},
			},
			Units: map[string]params.UnitStatus{
				"wordpress/0": {
					WorkloadStatus: params.DetailedStatus{
						Status: "error",
						Info:   "blam",
						Data:   map[string]interface{}{"relation-id": "0"},
					},
					AgentStatus: params.DetailedStatus{
						Status: "idle",
					},
					Machine: "1",
					Subordinates: map[string]params.UnitStatus{
						"logging/0": {
							WorkloadStatus: params.DetailedStatus{
								Status: "waiting",
								Info:   "waiting for machine",
							},
							AgentStatus: params.DetailedStatus{
								Status: "allocating",
							},
						},
					},
				},
				"wordpress/1": {
					WorkloadStatus: params.DetailedStatus{
						Status: "waiting",
						Info:   "waiting for machine",
					},
					AgentStatus: params.DetailedStatus{
						Status: "allocating",
						Info:   "",
					},

					Machine: "2",
					Subordinates: map[string]params.UnitStatus{
						"logging/1": {
							WorkloadStatus: params.DetailedStatus{
								Status: "waiting",
								Info:   "waiting for machine",
							},
							AgentStatus: params.DetailedStatus{
								Status: "allocating",
								Info:   "",
							},
						},
					},
				},
			},
			EndpointBindings: map[string]string{
				"":                network.AlphaSpaceName,
				"foo-bar":         network.AlphaSpaceName,
				"logging-dir":     network.AlphaSpaceName,
				"monitoring-port": network.AlphaSpaceName,
				"url":             network.AlphaSpaceName,
				"admin-api":       network.AlphaSpaceName,
				"cache":           network.AlphaSpaceName,
				"db":              network.AlphaSpaceName,
				"db-client":       network.AlphaSpaceName,
			},
		},
	},
	Relations: []params.RelationStatus{
		{
			Id:  0,
			Key: "logging:logging-directory wordpress:logging-dir",
			Endpoints: []params.EndpointStatus{
				{
					ApplicationName: "logging",
					Name:            "logging-directory",
					Role:            "requirer",
					Subordinate:     true,
				},
				{
					ApplicationName: "wordpress",
					Name:            "logging-dir",
					Role:            "provider",
					Subordinate:     false,
				},
			},
			Interface: "logging",
			Scope:     "container",
			Status: params.DetailedStatus{
				Status: "joining",
				Info:   "",
			},
		},
	},
	Branches: map[string]params.BranchStatus{},
}

// setUpScenario makes a model scenario suitable for
// testing most kinds of access scenario. It returns
// a list of all the entities in the scenario.
//
// When the scenario is initialized, we have:
// user-admin
// user-other
// machine-0
//  instance-id="i-machine-0"
//  nonce="fake_nonce"
//  jobs=manage-environ
//  status=started, info=""
// machine-1
//  instance-id="i-machine-1"
//  nonce="fake_nonce"
//  jobs=host-units
//  status=started, info=""
//  constraints=mem=1G
// machine-2
//  instance-id="i-machine-2"
//  nonce="fake_nonce"
//  jobs=host-units
//  status=started, info=""
// application-wordpress
// application-logging
// unit-wordpress-0
//  deployer-name=machine-1
//  status=down with error and status data attached
// unit-logging-0
//  deployer-name=unit-wordpress-0
// unit-wordpress-1
//     deployer-name=machine-2
// unit-logging-1
//  deployer-name=unit-wordpress-1
// remoteapplication-mediawiki
// applicationoffer-hosted-mysql
//
// The passwords for all returned entities are
// set to the entity name with a " password" suffix.
//
// Note that there is nothing special about machine-0
// here - it's the controller in this scenario
// just because machine 0 has traditionally been the
// controller (bootstrap machine), so is
// hopefully easier to remember as such.
func (s *baseSuite) setUpScenario(c *gc.C) (entities []names.Tag) {
	add := func(e state.Entity) {
		entities = append(entities, e.Tag())
	}
	u, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	setDefaultPassword(c, u)
	add(u)
	err = s.Model.UpdateModelConfig(map[string]interface{}{
		config.AgentVersionKey: "1.2.3"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	u = s.Factory.MakeUser(c, &factory.UserParams{Name: "other"})
	setDefaultPassword(c, u)
	add(u)

	m, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, names.NewMachineTag("0"))
	err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	setDefaultPassword(c, m)
	setDefaultStatus(c, m)
	add(m)
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-db2",
		OfferUUID:   "offer-uuid",
		URL:         "admin/prod.db2",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{
			{
				Name:      "database",
				Interface: "db2",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "mediawiki",
		SourceModel:     coretesting.ModelTag,
		IsConsumerProxy: true,
		Endpoints: []charm.Relation{
			{
				Name:      "db",
				Interface: "mysql",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	eps, err = s.State.InferEndpoints("mediawiki", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	mwRel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
		Endpoints:       map[string]string{"database": "server"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: coretesting.ModelTag.Id(),
		Username:        "fred",
		OfferUUID:       offer.OfferUUID,
		RelationId:      mwRel.Id(),
		RelationKey:     mwRel.Tag().Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(wu.Tag(), gc.Equals, names.NewUnitTag(fmt.Sprintf("wordpress/%d", i)))
		setDefaultPassword(c, wu)
		add(wu)

		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Tag(), gc.Equals, names.NewMachineTag(fmt.Sprintf("%d", i+1)))
		if i == 1 {
			err = m.SetConstraints(constraints.MustParse("mem=1G"))
			c.Assert(err, jc.ErrorIsNil)
		}
		err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		setDefaultPassword(c, m)
		setDefaultStatus(c, m)
		add(m)

		err = wu.AssignToMachine(m)
		c.Assert(err, jc.ErrorIsNil)

		wru, err := rel.Unit(wu)
		c.Assert(err, jc.ErrorIsNil)

		// Put wordpress/0 in error state (with extra status data set)
		if i == 0 {
			sd := map[string]interface{}{
				"relation-id": "0",
				// these this should get filtered out
				// (not in StatusData whitelist)
				"remote-unit": "logging/0",
				"foo":         "bar",
			}
			now := time.Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "blam",
				Data:    sd,
				Since:   &now,
			}
			err := wu.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)
		}

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)

		lu, err := s.State.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lu.IsPrincipal(), jc.IsFalse)
		setDefaultPassword(c, lu)
		add(lu)
	}
	return
}
