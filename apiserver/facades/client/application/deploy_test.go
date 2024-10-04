// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployApplication demands that a charm already exists in state,
// and that is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.ApiServerSuite
	charm *state.Charm
}

var _ = gc.Suite(&DeployLocalSuite{})

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *DeployLocalSuite) modelConfigService(c *gc.C) application.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *DeployLocalSuite) SetUpSuite(c *gc.C) {
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	curl := charm.MustParseURL("local:quantal/dummy")
	ch := testcharms.RepoForSeries("quantal").CharmDir("dummy")
	charm, err := testing.PutCharm(s.ControllerModel(c).State(), s.ObjectStore(c, s.ControllerModelUUID()), curl, ch)
	c.Assert(err, jc.ErrorIsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployControllerNotAllowed(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	ch := f.MakeCharm(c, &factory.CharmParams{Name: "juju-controller"})
	_, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State(), modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "my-controller",
			Charm:           ch,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, gc.ErrorMatches, "manual deploy of the controller charm not supported")
}

func (s *DeployLocalSuite) TestDeployMinimal(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State(), modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharm(c, app, s.charm.URL())
	s.assertSettings(c, app, charm.Settings{})
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes{})
	s.assertConstraints(c, app, constraints.MustParse("arch=amd64"))
	s.assertMachines(c, app, constraints.Value{})
}

func (s *DeployLocalSuite) TestDeployChannel(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.CharmOrigin, jc.DeepEquals, &state.CharmOrigin{
		Source:   "local",
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"}})
}

func (s *DeployLocalSuite) TestDeployWithImplicitBindings(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State(), modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName:  "bob",
			Charm:            wordpressCharm,
			EndpointBindings: nil,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"}},
		},
		loggertesting.WrapCheckLog(c),
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
	wordpressCharm, err := testing.PutCharm(s.ControllerModel(c).State(), s.ObjectStore(c, s.ControllerModelUUID()), charmURL, ch)
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
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	wordpressCharm := s.addWordpressCharm(c)
	st := s.ControllerModel(c).State()
	// dbSpace, err := st.AddSpace("db", "", nil)
	// c.Assert(err, jc.ErrorIsNil)
	// publicSpace, err := st.AddSpace("public", "", nil)
	// c.Assert(err, jc.ErrorIsNil)
	dbSpaceId := "db-space"
	publicSpaceId := "public-space"

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st, modelConfigService: s.modelConfigService(c)},
		m,
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   publicSpaceId,
				"db": dbSpaceId,
			},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		// default binding
		"": publicSpaceId,
		// relation names
		"url":             publicSpaceId,
		"logging-dir":     publicSpaceId,
		"monitoring-port": publicSpaceId,
		"db":              dbSpaceId,
		"cache":           publicSpaceId,
		// extra-bindings names
		"db-client": publicSpaceId,
		"admin-api": publicSpaceId,
		"foo-bar":   publicSpaceId,
	})
}

func (s *DeployLocalSuite) TestDeployWithBoundRelationNamesAndExtraBindingsNames(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)
	st := s.ControllerModel(c).State()
	// dbSpace, err := st.AddSpace("db", "", nil)
	// c.Assert(err, jc.ErrorIsNil)
	// publicSpace, err := st.AddSpace("public", "", nil)
	// c.Assert(err, jc.ErrorIsNil)
	// internalSpace, err := st.AddSpace("internal", "", nil)
	// c.Assert(err, jc.ErrorIsNil)
	dbSpaceId := "db-space"
	publicSpaceId := "public-space"
	internalSpaceId := "internal-space"

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st, modelConfigService: s.modelConfigService(c)},
		m,
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":          publicSpaceId,
				"db":        dbSpaceId,
				"db-client": dbSpaceId,
				"admin-api": internalSpaceId,
			},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"":                publicSpaceId,
		"url":             publicSpaceId,
		"logging-dir":     publicSpaceId,
		"monitoring-port": publicSpaceId,
		"db":              dbSpaceId,
		"cache":           publicSpaceId,
		"db-client":       dbSpaceId,
		"admin-api":       internalSpaceId,
		"cluster":         publicSpaceId,
		"foo-bar":         publicSpaceId, // like for relations, uses the application-default.
	})

}

func (s *DeployLocalSuite) TestDeployResources(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			EndpointBindings: map[string]string{
				"": "public",
			},
			Resources: map[string]string{"foo": "bar"},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Resources, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *DeployLocalSuite) TestDeploySettings(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State(), modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"title":       "banana cupcakes",
				"skill-level": 9901,
			},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSettings(c, app, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	st := s.ControllerModel(c).State()
	_, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st, modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"skill-level": 99.01,
			},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
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
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	cfg, err := coreconfig.NewConfig(map[string]interface{}{
		"outlook":     "good",
		"skill-level": 1,
	}, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: s.ControllerModel(c).State(), modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName:   "bob",
			Charm:             s.charm,
			ApplicationConfig: cfg,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationConfig(c, app, coreconfig.ConfigAttributes{
		"outlook":     "good",
		"skill-level": 1,
	})
}

func (s *DeployLocalSuite) TestDeployConstraints(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	st := s.ControllerModel(c).State()
	err := st.SetModelConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	applicationCons := constraints.MustParse("cores=2")

	app, err := application.DeployApplication(
		context.Background(),
		stateDeployer{State: st, modelConfigService: s.modelConfigService(c)},
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, app, constraints.MustParse("cores=2 arch=amd64"))
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        2,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 2)
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
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
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement(fmt.Sprintf("%s:0", instance.LXD))},
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
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
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

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
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        4,
			Placement:       placement,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 4)
	c.Assert(f.args.Placement, gc.DeepEquals, placement)
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirements(c *gc.C) {
	s.ProviderTracker = fakeProviderTracker{}

	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	st := s.ControllerModel(c).State()
	objectStore := s.ObjectStore(c, s.ControllerModelUUID())
	charm, err := testing.PutCharm(st, objectStore, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	var f = fakeDeployer{}

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = application.DeployApplication(
		context.Background(),
		&f,
		m,
		model.ReadOnlyModel{
			UUID: model.UUID(s.ControllerModelUUID()),
		},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, gc.ErrorMatches, `(?s)Charm cannot be deployed because:
  - charm requires Juju version >= 42.0.0.*`)
}

func (s *DeployLocalSuite) TestDeployWithUnmetCharmRequirementsAndForce(c *gc.C) {
	s.ProviderTracker = fakeProviderTracker{}

	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	curl := charm.MustParseURL("local:focal/juju-qa-test-assumes-v2")
	ch := testcharms.Hub.CharmDir("juju-qa-test-assumes-v2")
	st := s.ControllerModel(c).State()
	objectStore := s.ObjectStore(c, s.ControllerModelUUID())
	charm, err := testing.PutCharm(st, objectStore, curl, ch)
	c.Assert(err, jc.ErrorIsNil)

	var f = fakeDeployer{}

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = application.DeployApplication(
		context.Background(),
		&f,
		m,
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "assume-metal",
			Charm:           charm,
			NumUnits:        1,
			Force:           true, // bypass assumes checks
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeployLocalSuite) TestDeployWithFewerPlacement(c *gc.C) {
	domainServices := s.DefaultModelDomainServices(c)
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: provider.CommonStorageProviders(),
		Secrets:         service.NotImplementedSecretService{},
	})

	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{{Scope: s.ControllerModelUUID(), Directive: "valid"}}
	_, err := application.DeployApplication(
		context.Background(),
		&f,
		s.ControllerModel(c),
		model.ReadOnlyModel{},
		applicationService,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        3,
			Placement:       placement,
			CharmOrigin: corecharm.Origin{
				Source:   corecharm.Local,
				Platform: corecharm.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"},
			},
		},
		loggertesting.WrapCheckLog(c),
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
	settings, err := app.CharmConfig()
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
		res, err := st.AssignStagedUnits(s.modelConfigService(c), nil, []string{id})
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
	modelConfigService application.ModelConfigService
}

func (d stateDeployer) ReadSequence(name string) (int, error) {
	return state.ReadSequence(d.State, name)
}

func (d stateDeployer) AddApplication(args state.AddApplicationArgs, store objectstore.ObjectStore) (application.Application, error) {
	app, err := d.State.AddApplication(d.modelConfigService, args, store)
	if err != nil {
		return nil, err
	}
	return application.NewStateApplication(d.State, d.modelConfigService, app), nil
}

type fakeDeployer struct {
	args state.AddApplicationArgs
}

func (f *fakeDeployer) ReadSequence(name string) (int, error) {
	return 0, nil
}

func (f *fakeDeployer) AddApplication(args state.AddApplicationArgs, _ objectstore.ObjectStore) (application.Application, error) {
	f.args = args
	return nil, nil
}

type fakeProviderTracker struct{}

func (fakeProviderTracker) ProviderForModel(ctx context.Context, namespace string) (providertracker.Provider, error) {
	return fakeProvider{}, nil
}

type fakeProvider struct {
	providertracker.Provider
}

func (fakeProvider) SupportedFeatures() (assumes.FeatureSet, error) {
	return assumes.FeatureSet{}, nil
}
