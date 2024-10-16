// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/core/network"
	applicationservice "github.com/juju/juju/domain/application/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	testfactory "github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type filteringStatusSuite struct {
	baseSuite
}

var _ = gc.Suite(&filteringStatusSuite{})

func (s *filteringStatusSuite) TestRelationFiltered(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// make application 1 with endpoint 1
	a1 := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Name: "abc",
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := a1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	// make application 2 with endpoint 2
	a2 := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Name: "def",
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := a2.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between a1 and a2
	r12 := factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(r12, gc.NotNil)

	// create another application 3 with an endpoint 3
	a3 := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging"}),
	})
	e3, err := a3.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	// create endpoint 4 on application 1
	e4, err := a1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	r13 := factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{e3, e4},
	})
	c.Assert(r13, gc.NotNil)

	// Test status filtering with application 1: should get both relations
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{a1.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a1.Name(), 2, status.Relations)

	// test status filtering with application 3: should get 1 relation
	status, err = client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{a3.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a3.Name(), 1, status.Relations)
}

// TestApplicationFilterIndependentOfAlphabeticUnitOrdering ensures we
// do not regress and are carrying forward fix for lp#1592872.
func (s *filteringStatusSuite) TestApplicationFilterIndependentOfAlphabeticUnitOrdering(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
		Name: "abc",
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
		Name: "def",
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	for i := 0; i < 20; i++ {
		c.Logf("run %d", i)
		status, err := client.Status(
			context.Background(),
			&apiclient.StatusArgs{
				Patterns: []string{applicationA.Name()},
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(status.Applications, gc.HasLen, 2)
	}
}

// TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly
// tests scenario where applications are returned as part of the status because
// they are related to an application that matches given filter.
// However, the relations for these applications should not be returned.
// In other words, if there are two applications, A and B, such that:
//
// * an application A matches the supplied filter directly;
// * an application B has units on the same machine as units of an application A and, thus,
// qualifies to be returned by the status result;
//
// application B's relations should not be returned.
func (s *filteringStatusSuite) TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging"}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	// Filtering status on application A should get:
	// * no relations;
	// * two applications.
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{applicationA.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Check(status.Applications, gc.HasLen, 2)
	c.Check(status.Relations, gc.HasLen, 0)
}

func (s *filteringStatusSuite) TestFilterByPortRange(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))

	app := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	_ = factory.MakeUnit(c, &testfactory.UnitParams{
		Application: app,
	})
	_ = factory.MakeUnit(c, &testfactory.UnitParams{
		Application: app,
	})

	appService := s.ControllerDomainServices(c).Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         applicationservice.NotImplementedSecretService{},
	})

	unit0UUID, err := appService.GetUnitUUID(context.Background(), "wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	unit1UUID, err := appService.GetUnitUUID(context.Background(), "wordpress/1")
	c.Assert(err, jc.ErrorIsNil)

	portService := s.ControllerDomainServices(c).Port()
	err = portService.UpdateUnitPorts(context.Background(), unit0UUID, network.GroupedPortRanges{
		"":    []network.PortRange{network.MustParsePortRange("1000/tcp")},
		"foo": []network.PortRange{network.MustParsePortRange("2000/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	err = portService.UpdateUnitPorts(context.Background(), unit1UUID, network.GroupedPortRanges{
		"":    []network.PortRange{network.MustParsePortRange("2000/tcp")},
		"bar": []network.PortRange{network.MustParsePortRange("3000/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))

	status, err := client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{"1000/tcp"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Check(status.Applications, jc.Satisfies, func(apps map[string]params.ApplicationStatus) bool {
		_, ok0 := apps[app.Name()].Units["wordpress/0"]
		_, ok1 := apps[app.Name()].Units["wordpress/1"]
		return ok0 && !ok1
	})

	status, err = client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{"2000/tcp"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	c.Check(status.Applications, jc.Satisfies, func(apps map[string]params.ApplicationStatus) bool {
		_, ok0 := apps[app.Name()].Units["wordpress/0"]
		_, ok1 := apps[app.Name()].Units["wordpress/1"]
		return ok0 && ok1
	})

	status, err = client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{"3000/tcp"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Check(status.Applications, jc.Satisfies, func(apps map[string]params.ApplicationStatus) bool {
		_, ok0 := apps[app.Name()].Units["wordpress/0"]
		_, ok1 := apps[app.Name()].Units["wordpress/1"]
		return !ok0 && ok1
	})
}
