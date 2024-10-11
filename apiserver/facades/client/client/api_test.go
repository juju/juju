// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/charm"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type baseSuite struct {
	testing.ApiServerSuite
}

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *baseSuite) modelConfigService(c *gc.C) state.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithLeaseManager = true
	s.ApiServerSuite.SetUpTest(c)
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
	err := e.SetPassword(defaultPassword(e.Tag()))
	c.Assert(err, jc.ErrorIsNil)
}

func defaultPassword(tag names.Tag) string {
	return tag.String() + " password-1234567890"
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
	info := s.ControllerModelApiInfo()
	info.Tag = tag
	info.Password = defaultPassword(tag)
	// Set this always, so that the login attempts as a machine will
	// not fail with ErrNotProvisioned; it's not used otherwise.
	info.Nonce = "fake_nonce"

	c.Logf("opening state; entity %q; password %q", info.Tag, info.Password)
	st, err := api.Open(context.Background(), info, api.DialOpts{})
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
		CloudTag:    "cloud-" + testing.DefaultCloud.Name,
		CloudRegion: testing.DefaultCloudRegion,
		Version:     version.Current.String(),
		ModelStatus: params.DetailedStatus{
			Status: "available",
		},
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
			Base:       params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
			Base:       params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
			Base:        params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
			Charm: "local:quantal/logging-1",
			Base:  params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
			Base:          params.Base{Name: "ubuntu", Channel: "12.10/stable"},
			Relations:     map[string][]string{},
			SubordinateTo: []string{},
			Units:         map[string]params.UnitStatus{},
			// Since there are no units, the derived status is Unknown.
			Status: params.DetailedStatus{
				Status: "unknown",
			},
			EndpointBindings: map[string]string{
				"":               network.AlphaSpaceName,
				"server":         network.AlphaSpaceName,
				"server-admin":   network.AlphaSpaceName,
				"db-router":      network.AlphaSpaceName,
				"metrics-client": network.AlphaSpaceName,
			},
		},
		"wordpress": {
			Charm: "local:quantal/wordpress-3",
			Base:  params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
}

// setUpScenario makes a model scenario suitable for
// testing most kinds of access scenario. It returns
// a list of all the entities in the scenario.
//
// When the scenario is initialized, we have:
// user-admin
// user-other
// machine-0
//
//	instance-id="i-machine-0"
//	nonce="fake_nonce"
//	jobs=manage-environ
//	status=started, info=""
//
// machine-1
//
//	instance-id="i-machine-1"
//	nonce="fake_nonce"
//	jobs=host-units
//	status=started, info=""
//	constraints=mem=1G
//
// machine-2
//
//	instance-id="i-machine-2"
//	nonce="fake_nonce"
//	jobs=host-units
//	status=started, info=""
//
// application-wordpress
// application-logging
// unit-wordpress-0
//
//	deployer-name=machine-1
//	status=down with error and status data attached
//
// unit-logging-0
//
//	deployer-name=unit-wordpress-0
//
// unit-wordpress-1
//
//	deployer-name=machine-2
//
// unit-logging-1
//
//	deployer-name=unit-wordpress-1
//
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
	st := s.ControllerModel(c).State()
	domainServices := s.ControllerDomainServices(c)
	accessService := domainServices.Access()
	modelConfigService := domainServices.Config()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(modelConfigService)

	add := func(e state.Entity) {
		entities = append(entities, e.Tag())
	}

	// Add the admin user.
	adminPassword := defaultPassword(testing.AdminUser)
	err := accessService.SetPassword(context.Background(), usertesting.GenNewName(c, testing.AdminUser.Name()), auth.NewPassword(adminPassword))
	c.Assert(err, jc.ErrorIsNil)
	add(taggedUser{tag: testing.AdminUser})

	err = s.ControllerModel(c).UpdateModelConfig(
		s.ConfigSchemaSourceGetter(c),
		map[string]interface{}{
			config.AgentVersionKey: "2.0.0",
		}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add another user.
	userTag := names.NewUserTag("other")
	userPassword := defaultPassword(userTag)
	_, _, err = accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        usertesting.GenNewName(c, userTag.Name()),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword(userPassword)),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = accessService.CreatePermission(context.Background(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.AdminAccess,
		},
		User: usertesting.GenNewName(c, userTag.Name()),
	})
	c.Assert(err, jc.ErrorIsNil)

	add(taggedUser{tag: userTag})

	machineService := s.ControllerDomainServices(c).Machine()
	machineUUID, err := machineService.CreateMachine(context.Background(), machine.Name("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machineUUID, instance.Id("i-machine-0"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// We are still double writing machines and their provisioning info.
	m, err := st.AddMachine(modelConfigService, state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, names.NewMachineTag("0"))
	err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	setDefaultPassword(c, m)
	setDefaultStatus(c, m)
	add(m)

	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "local:quantal/mysql-1"})
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source:   "local",
			Platform: &state.Platform{OS: "ubuntu", Channel: "12.10/stable", Architecture: "amd64"},
		},
	})
	wpch := f.MakeCharm(c, &factory.CharmParams{Name: "wordpress", URL: "local:quantal/wordpress-3"})
	wordpress := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: wpch,
		CharmOrigin: &state.CharmOrigin{
			Source:   "local",
			Platform: &state.Platform{OS: "ubuntu", Channel: "12.10/stable", Architecture: "amd64"},
		},
	})
	loggingch := f.MakeCharm(c, &factory.CharmParams{Name: "logging", URL: "local:quantal/logging-1"})
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingch,
		CharmOrigin: &state.CharmOrigin{
			Source:   "local",
			Platform: &state.Platform{OS: "ubuntu", Channel: "12.10/stable", Architecture: "amd64"},
		},
	})
	eps, err := st.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.AddRemoteApplication(state.AddRemoteApplicationParams{
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
	_, err = st.AddRemoteApplication(state.AddRemoteApplicationParams{
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
	eps, err = st.InferEndpoints("mediawiki", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	mwRel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	offers := state.NewApplicationOffers(st)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
		Endpoints:       map[string]string{"database": "server"},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: coretesting.ModelTag.Id(),
		Username:        "fred",
		OfferUUID:       offer.OfferUUID,
		RelationId:      mwRel.Id(),
		RelationKey:     mwRel.Tag().Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit(modelConfigService, state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(wu.Tag(), gc.Equals, names.NewUnitTag(fmt.Sprintf("wordpress/%d", i)))
		setDefaultPassword(c, wu)
		add(wu)

		machineService := s.ControllerDomainServices(c).Machine()
		machineUUID, err := machineService.CreateMachine(context.Background(), machine.Name(fmt.Sprintf("%d", i+1)))
		c.Assert(err, jc.ErrorIsNil)
		err = machineService.SetMachineCloudInstance(context.Background(), machineUUID, instance.Id(fmt.Sprintf("i-machine-%d", i+1)), "", nil)
		c.Assert(err, jc.ErrorIsNil)
		// We are still double writing machines and their provisioning info.
		m, err := st.AddMachine(modelConfigService, state.UbuntuBase("12.10"), state.JobHostUnits)
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

		err = wu.AssignToMachine(modelConfigService, m)
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
		err = wru.EnterScope(modelConfigService, nil)
		c.Assert(err, jc.ErrorIsNil)

		lu, err := st.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lu.IsPrincipal(), jc.IsFalse)
		setDefaultPassword(c, lu)
		add(lu)
	}
	return
}

type taggedUser struct {
	tag names.Tag
}

func (u taggedUser) Tag() names.Tag {
	return u.tag
}

func ptr[T any](t T) *T {
	return &t
}
