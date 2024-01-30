// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"

	"github.com/juju/charm/v13"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/juju/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployApplication demands that a charm already exists in state,
// and that is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.ApiServerSuite
	charm *state.Charm
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpSuite(c *gc.C) {
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	curl := charm.MustParseURL("local:quantal/dummy")
	ch := testcharms.RepoForSeries("quantal").CharmDir("dummy")
	charm, err := testing.PutCharm(s.ControllerModel(c).State(), curl, ch)
	c.Assert(err, jc.ErrorIsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployControllerNotAllowed(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	serviceFactory := s.DefaultModelServiceFactory(c)

	ch := f.MakeCharm(c, &factory.CharmParams{Name: "juju-controller"})
	_, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State()},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "my-controller",
			Charm:           ch,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "manual deploy of the controller charm not supported")
}

func (s *DeployLocalSuite) TestDeployMinimal(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State()},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharm(c, app, s.charm.URL())
	s.assertSettings(c, app, charm.Settings{})
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes(nil))
	s.assertConstraints(c, app, constraints.MustParse("arch=amd64"))
	s.assertMachines(c, app, constraints.Value{})
}

func (s *DeployLocalSuite) TestDeployChannel(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.CharmOrigin, jc.DeepEquals, &state.CharmOrigin{
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}})
}

func (s *DeployLocalSuite) TestDeployWithImplicitBindings(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State()},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName:  "bob",
			Charm:            wordpressCharm,
			EndpointBindings: nil,
			CharmOrigin:      corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"": network.AlphaSpaceId,
		// relation names
		"url":             network.AlphaSpaceId,
		"logging-dir":     network.AlphaSpaceId,
		"monitoring-port": network.AlphaSpaceId,
		"db":              network.AlphaSpaceId,
		"cache":           network.AlphaSpaceId,
		"cluster":         network.AlphaSpaceId,
		// extra-bindings names
		"db-client": network.AlphaSpaceId,
		"admin-api": network.AlphaSpaceId,
		"foo-bar":   network.AlphaSpaceId,
	})
}

func (s *DeployLocalSuite) addWordpressCharm(c *gc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmWithExtraBindings(c *gc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress-extra-bindings")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmFromURL(c *gc.C, charmURL *charm.URL) *state.Charm {
	ch := testcharms.RepoForSeries("quantal").CharmDir(charmURL.Name)
	wordpressCharm, err := testing.PutCharm(s.ControllerModel(c).State(), charmURL, ch)
	c.Assert(err, jc.ErrorIsNil)
	return wordpressCharm
}

func (s *DeployLocalSuite) assertBindings(c *gc.C, app application.Application, expected map[string]string) {
	type withEndpointBindings interface {
		EndpointBindings() (application.Bindings, error)
	}
	bindings, err := app.(withEndpointBindings).EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings.Map(), jc.DeepEquals, expected)
}

func (s *DeployLocalSuite) TestDeployWithSomeSpecifiedBindings(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	wordpressCharm := s.addWordpressCharm(c)
	st := s.ControllerModel(c).State()
	dbSpace, err := st.AddSpace("db", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	publicSpace, err := st.AddSpace("public", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st},
		model,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   publicSpace.Id(),
				"db": dbSpace.Id(),
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		// default binding
		"": publicSpace.Id(),
		// relation names
		"url":             publicSpace.Id(),
		"logging-dir":     publicSpace.Id(),
		"monitoring-port": publicSpace.Id(),
		"db":              dbSpace.Id(),
		"cache":           publicSpace.Id(),
		// extra-bindings names
		"db-client": publicSpace.Id(),
		"admin-api": publicSpace.Id(),
		"foo-bar":   publicSpace.Id(),
	})
}

func (s *DeployLocalSuite) TestDeployWithBoundRelationNamesAndExtraBindingsNames(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)
	st := s.ControllerModel(c).State()
	dbSpace, err := st.AddSpace("db", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	publicSpace, err := st.AddSpace("public", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	internalSpace, err := st.AddSpace("internal", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st},
		model,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":          publicSpace.Id(),
				"db":        dbSpace.Id(),
				"db-client": dbSpace.Id(),
				"admin-api": internalSpace.Id(),
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"":                publicSpace.Id(),
		"url":             publicSpace.Id(),
		"logging-dir":     publicSpace.Id(),
		"monitoring-port": publicSpace.Id(),
		"db":              dbSpace.Id(),
		"cache":           publicSpace.Id(),
		"db-client":       dbSpace.Id(),
		"admin-api":       internalSpace.Id(),
		"cluster":         publicSpace.Id(),
		"foo-bar":         publicSpace.Id(), // like for relations, uses the application-default.
	})

}

func (s *DeployLocalSuite) TestDeployWithInvalidSpace(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	wordpressCharm := s.addWordpressCharm(c)
	st := s.ControllerModel(c).State()
	_, err := st.AddSpace("db", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	publicSpace, err := st.AddSpace("public", "", nil)
	c.Assert(err, jc.ErrorIsNil)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st},
		model,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   publicSpace.Id(),
				"db": "42", // unknown space id
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, gc.ErrorMatches, `cannot add application "bob": space not found`)
	c.Check(app, gc.IsNil)
	// The application should not have been added
	_, err = st.Application("bob")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *DeployLocalSuite) TestDeployResources(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			EndpointBindings: map[string]string{
				"": "public",
			},
			Resources:   map[string]string{"foo": "bar"},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Resources, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *DeployLocalSuite) TestDeploySettings(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State()},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"title":       "banana cupcakes",
				"skill-level": 9901,
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSettings(c, app, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	st := s.ControllerModel(c).State()
	_, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"skill-level": 99.01,
			},
			CharmOrigin: corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got 99.01`)
	_, err = st.Application("bob")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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

func (s *DeployLocalSuite) TestDeployWithApplicationConfig(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	cfg, err := coreconfig.NewConfig(map[string]interface{}{
		"outlook":     "good",
		"skill-level": 1,
	}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State()},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName:   "bob",
			Charm:             s.charm,
			ApplicationConfig: cfg,
			CharmOrigin:       corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes{
		"outlook":     "good",
		"skill-level": 1,
	})
}

func (s *DeployLocalSuite) TestDeployConstraints(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	st := s.ControllerModel(c).State()
	err := st.SetModelConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	applicationCons := constraints.MustParse("cores=2")

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st},
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, app, constraints.MustParse("cores=2 arch=amd64"))
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        2,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 2)
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 1)
	c.Assert(f.args.Placement, gc.HasLen, 1)
	c.Assert(*f.args.Placement[0], gc.Equals, instance.Placement{Scope: instance.MachineScope, Directive: "0"})
}

func (s *DeployLocalSuite) TestDeployForceMachineIdWithContainer(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement(fmt.Sprintf("%s:0", instance.LXD))},
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 1)
	c.Assert(f.args.Placement, gc.HasLen, 1)
	c.Assert(*f.args.Placement[0], gc.Equals, instance.Placement{Scope: string(instance.LXD), Directive: "0"})
}

func (s *DeployLocalSuite) TestDeploy(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{
		{Scope: s.ControllerModelUUID(), Directive: "valid"},
		{Scope: "#", Directive: "0"},
		{Scope: "lxd", Directive: "1"},
		{Scope: "lxd", Directive: ""},
	}
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        4,
			Placement:       placement,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 4)
	c.Assert(f.args.Placement, gc.DeepEquals, placement)
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirements(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	st := s.ControllerModel(c).State()
	charm, err := testing.PutCharm(st, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	var f = fakeDeployer{}

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = application.DeployApplication(
		context.Background(),
		&f,
		model,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "(?m).*Charm feature requirements cannot be met.*")
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirementsAndForce(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	st := s.ControllerModel(c).State()
	charm, err := testing.PutCharm(st, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	var f = fakeDeployer{}

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = application.DeployApplication(
		context.Background(),
		&f,
		model,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			Force:           true, // bypass assumes checks
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployLocalSuite) TestDeployWithFewerPlacement(c *gc.C) {
	serviceFactory := s.DefaultModelServiceFactory(c)

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{{Scope: s.ControllerModelUUID(), Directive: "valid"}}
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Application(),
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        3,
			Placement:       placement,
			CharmOrigin:     corecharm.Origin{Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04"}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 3)
	c.Assert(f.args.Placement, gc.DeepEquals, placement)
}

func (s *DeployLocalSuite) assertCharm(c *gc.C, app application.Application, expect string) {
	curl, force := app.CharmURL()
	c.Assert(*curl, gc.Equals, expect)
	c.Assert(force, jc.IsFalse)
}

func (s *DeployLocalSuite) assertSettings(c *gc.C, app application.Application, _ charm.Settings) {
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	expected := s.charm.Config().DefaultSettings()
	for name, value := range settings {
		expected[name] = value
	}
	c.Assert(settings, gc.DeepEquals, expected)
}

func (s *DeployLocalSuite) assertApplicationConfig(c *gc.C, app application.Application, wantCfg coreconfig.ConfigAttributes) {
	cfg, err := app.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.DeepEquals, wantCfg)
}

func (s *DeployLocalSuite) assertConstraints(c *gc.C, app application.Application, expect constraints.Value) {
	cons, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertMachines(c *gc.C, app application.Application, expectCons constraints.Value, expectIds ...string) {
	type withAssignedMachineId interface {
		AssignedMachineId() (string, error)
	}

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	// first manually tell state to assign all the units
	st := s.ControllerModel(c).State()
	for _, unit := range units {
		id := unit.UnitTag().Id()
		res, err := st.AssignStagedUnits([]string{id})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(res[0].Error, jc.ErrorIsNil)
		c.Assert(res[0].Unit, gc.Equals, id)
	}

	// refresh the list of units from state
	units, err = app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	unseenIds := set.NewStrings(expectIds...)
	for _, unit := range units {
		id, err := unit.(withAssignedMachineId).AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		unseenIds.Remove(id)
		machine, err := st.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		cons, err := machine.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cons, gc.DeepEquals, expectCons)
	}
	c.Assert(unseenIds, gc.DeepEquals, set.NewStrings())
}

type stateDeployer struct {
	*state.State
}

func (d stateDeployer) AddApplication(args state.AddApplicationArgs, as application.ApplicationSaver, store objectstore.ObjectStore) (application.Application, error) {
	app, err := d.State.AddApplication(args, as, store)
	if err != nil {
		return nil, err
	}
	return application.NewStateApplication(d.State, app), nil
}

type fakeDeployer struct {
	args          state.AddApplicationArgs
	controllerCfg *controller.Config
}

func (f *fakeDeployer) ControllerConfig() (controller.Config, error) {
	if f.controllerCfg != nil {
		return *f.controllerCfg, nil
	}
	return controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, map[string]interface{}{})
}

func (f *fakeDeployer) AddApplication(args state.AddApplicationArgs, _ application.ApplicationSaver, _ objectstore.ObjectStore) (application.Application, error) {
	f.args = args
	return nil, nil
}
