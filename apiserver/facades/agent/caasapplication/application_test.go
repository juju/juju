// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasapplication"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/services/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&CAASApplicationSuite{})

type CAASApplicationSuite struct {
	testing.DomainServicesSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasapplication.Facade
	st         *mockState
	clock      *testclock.Clock
	broker     *mockBroker

	applicationService *service.WatchableService
}

func (s *CAASApplicationSuite) SetUpTest(c *gc.C) {
	s.DomainServicesSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Now())

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}

	// Seed the model with an application, this will be used to allow the
	// upserting of units.
	domainServices := s.DefaultModelDomainServices(c)
	unitName := "gitlab/0"
	s.applicationService = domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	}

	_, err := s.applicationService.CreateApplication(
		context.Background(), "gitlab", &stubCharm{}, origin, service.AddApplicationArgs{
			ReferenceName: "gitlab",
		}, service.AddUnitArg{
			UnitName: unitName,
		})
	c.Assert(err, jc.ErrorIsNil)

	s.st = newMockState()
	s.broker = &mockBroker{}

	facade, err := caasapplication.NewFacade(
		s.resources,
		s.authorizer,
		s.st, s.st,
		s.ControllerDomainServices(c).ControllerConfig(),
		s.applicationService,
		s.ControllerDomainServices(c).Config(),
		&mockModelAgent{agentVersion: version.MustParse("1.9.99")},
		s.broker,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *CAASApplicationSuite) TestAddUnit(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.st.app.unit = &mockUnit{
		life: state.Alive,
		containerInfo: &mockCloudContainer{
			providerID: "gitlab-0",
			unit:       "gitlab/0",
		},
		updateOp: nil,
	}

	s.broker.app = &mockCAASApplication{
		units: []caas.Unit{{
			Id:      "gitlab-0",
			Address: "1.2.3.4",
			Ports:   []string{"80"},
			FilesystemInfo: []caas.FilesystemInfo{{
				Volume: caas.VolumeInfo{
					VolumeId: "vid",
				},
			}},
		}},
	}

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.Result.UnitName, gc.Equals, "gitlab/0")
	c.Assert(results.Result.AgentConf, gc.NotNil)

	s.st.CheckCallNames(c, "Model", "Application", "APIHostPortsForAgents")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "UpsertCAASUnit")

	mc := jc.NewMultiChecker()
	mc.AddExpr("_.AddUnitParams.PasswordHash", gc.Not(gc.IsNil))
	c.Assert(s.st.app.Calls()[0].Args[1], mc, state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			ProviderId: strPtr("gitlab-0"),
			UnitName:   strPtr("gitlab/0"),
			Address:    strPtr("1.2.3.4"),
			Ports:      &[]string{"80"},
		},
		OrderedScale:              true,
		OrderedId:                 0,
		ObservedAttachedVolumeIDs: []string{"vid"},
	})
}

func (s *CAASApplicationSuite) TestAddUnitNotNeeded(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.broker.app = &mockCAASApplication{
		units: []caas.Unit{{
			Id:      "gitlab-0",
			Address: "1.2.3.4",
			Ports:   []string{"80"},
		}},
	}
	s.st.app.SetErrors(errors.NotAssignedf("unrequired unit"))

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.ErrorMatches, "unrequired unit not assigned")

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "UpsertCAASUnit")
}

func (s *CAASApplicationSuite) TestReuseUnitByName(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.st.app.unit = &mockUnit{
		life: state.Alive,
		containerInfo: &mockCloudContainer{
			providerID: "gitlab-0",
			unit:       "gitlab/0",
		},
	}

	s.broker.app = &mockCAASApplication{
		units: []caas.Unit{{
			Id:      "gitlab-0",
			Address: "1.2.3.4",
			Ports:   []string{"80"},
		}},
	}

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.Result.UnitName, gc.Equals, "gitlab/0")
	c.Assert(results.Result.AgentConf, gc.NotNil)

	s.st.CheckCallNames(c, "Model", "Application", "APIHostPortsForAgents")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "UpsertCAASUnit")

	mc := jc.NewMultiChecker()
	mc.AddExpr("_.AddUnitParams.PasswordHash", gc.Not(gc.IsNil))
	c.Assert(s.st.app.Calls()[0].Args[1], mc, state.UpsertCAASUnitParams{
		AddUnitParams: state.AddUnitParams{
			ProviderId: strPtr("gitlab-0"),
			UnitName:   strPtr("gitlab/0"),
			Address:    strPtr("1.2.3.4"),
			Ports:      &[]string{"80"},
		},
		OrderedScale: true,
		OrderedId:    0,
	})
}

func (s *CAASApplicationSuite) TestDontReuseDeadUnitByName(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.st.app.SetErrors(errors.AlreadyExistsf("dead unit \"gitlab/0\""))

	s.broker.app = &mockCAASApplication{
		units: []caas.Unit{{
			Id:      "gitlab-0",
			Address: "1.2.3.4",
			Ports:   []string{"80"},
		}},
	}

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.ErrorMatches, `dead unit "gitlab/0" already exists`)

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "UpsertCAASUnit")
}

func (s *CAASApplicationSuite) TestFindByProviderID(c *gc.C) {
	c.Skip("skip for now, because of the TODO in UnitIntroduction facade: hardcoded deploymentType := caas.DeploymentStateful")

	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.st.app.unit = &mockUnit{
		life: state.Alive,
	}
	s.st.app.unit.SetErrors(errors.NotFoundf("cloud container"))

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.Result.UnitName, gc.Equals, "gitlab/0")
	c.Assert(results.Result.AgentConf, gc.NotNil)

	s.st.CheckCallNames(c, "Model", "Application", "ControllerConfig", "APIHostPortsForAgents")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "Life", "Charm", "AllUnits", "UpdateUnits")
	c.Assert(s.st.app.Calls()[3].Args[0], gc.DeepEquals, &state.UpdateUnitsOperation{
		Updates: []*state.UpdateUnitOperation{nil},
	})
}

func (s *CAASApplicationSuite) TestAgentConf(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	s.st.app.unit = &mockUnit{
		life: state.Alive,
		containerInfo: &mockCloudContainer{
			providerID: "gitlab-0",
			unit:       "gitlab/0",
		},
		updateOp: nil,
	}
	s.broker.app = &mockCAASApplication{
		units: []caas.Unit{{
			Id:      "gitlab-0",
			Address: "1.2.3.4",
			Ports:   []string{"80"},
		}},
	}

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.Result.UnitName, gc.Equals, "gitlab/0")
	c.Assert(results.Result.AgentConf, gc.NotNil)

	conf := map[string]interface{}{}
	err = yaml.Unmarshal(results.Result.AgentConf, conf)
	c.Assert(err, jc.ErrorIsNil)

	check := jc.NewMultiChecker()
	check.AddExpr(`_["cacert"]`, jc.Ignore)
	check.AddExpr(`_["oldpassword"]`, jc.Ignore)
	check.AddExpr(`_["values"]`, jc.Ignore)
	c.Assert(conf, check, map[string]interface{}{
		"tag":               "unit-gitlab-0",
		"datadir":           "/var/lib/juju",
		"transient-datadir": "/var/run/juju",
		"logdir":            "/var/log/juju",
		"metricsspooldir":   "/var/lib/juju/metricspool",
		"upgradedToVersion": "1.9.99",
		"cacert":            "ignore",
		"controller":        "controller-ffffffff-ffff-ffff-ffff-ffffffffffff",
		"model":             "model-ffffffff-ffff-ffff-ffff-ffffffffffff",
		"apiaddresses": []interface{}{
			"10.0.2.1:17070",
			"52.7.1.1:17070",
		},
		"oldpassword":               "ignore",
		"values":                    nil,
		"agent-logfile-max-backups": 0,
		"agent-logfile-max-size":    0,
	})
}

func (s *CAASApplicationSuite) TestDyingApplication(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
		PodUUID: "gitlab-uuid",
	}

	err := s.applicationService.DestroyApplication(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.ErrorMatches, `application not provisioned`)
}

func (s *CAASApplicationSuite) TestMissingArgUUID(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodName: "gitlab-0",
	}

	err := s.applicationService.DestroyApplication(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.ErrorMatches, `pod-uuid not valid`)
}

func (s *CAASApplicationSuite) TestMissingArgName(c *gc.C) {
	args := params.CAASUnitIntroductionArgs{
		PodUUID: "gitlab-uuid",
	}

	err := s.applicationService.DestroyApplication(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.facade.UnitIntroduction(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.ErrorMatches, `pod-name not valid`)
}

func (s *CAASApplicationSuite) TestUnitTerminatingAgentWillRestart(c *gc.C) {
	s.authorizer.Tag = names.NewUnitTag("gitlab/0")

	s.broker.app = &mockCAASApplication{
		state: caas.ApplicationState{
			DesiredReplicas: 1,
		},
	}

	s.st.units = map[string]*mockUnit{
		"gitlab/0": {
			life: state.Alive,
			containerInfo: &mockCloudContainer{
				providerID: "gitlab-0",
				unit:       "gitlab/0",
			},
			updateOp: nil,
		},
	}

	args := params.Entity{
		Tag: "unit-gitlab-0",
	}
	ctx := context.Background()
	results, err := s.facade.UnitTerminating(ctx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.WillRestart, jc.IsTrue)
}

func (s *CAASApplicationSuite) TestUnitTerminatingAgentDying(c *gc.C) {
	s.authorizer.Tag = names.NewUnitTag("gitlab/0")

	s.broker.app = &mockCAASApplication{
		state: caas.ApplicationState{
			DesiredReplicas: 0,
		},
	}

	s.st.units = map[string]*mockUnit{
		"gitlab/0": {
			life: state.Alive,
			containerInfo: &mockCloudContainer{
				providerID: "gitlab-0",
				unit:       "gitlab/0",
			},
			updateOp: nil,
		},
	}

	args := params.Entity{
		Tag: "unit-gitlab-0",
	}
	results, err := s.facade.UnitTerminating(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.WillRestart, jc.IsFalse)
}

func strPtr(s string) *string {
	return &s
}
