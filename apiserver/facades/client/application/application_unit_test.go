// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/assumes"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/schema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/application/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/controller"
	coreassumes "github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type mockLXDProfilerCharm struct {
	*mocks.MockCharm
	*mocks.MockLXDProfiler
}

type ApplicationSuite struct {
	testing.CleanupSuite

	api *application.APIBase

	backend            *mocks.MockBackend
	secrets            *mocks.MockSecretsStore
	storageAccess      *mocks.MockStorageInterface
	model              *mocks.MockModel
	leadershipReader   *mocks.MockReader
	storagePoolManager *mocks.MockPoolManager
	registry           *mocks.MockProviderRegistry
	caasBroker         *mocks.MockCaasBrokerInterface

	blockChecker  *mocks.MockBlockChecker
	changeAllowed error
	removeAllowed error

	authorizer apiservertesting.FakeAuthorizer
	modelType  state.ModelType

	allSpaceInfos network.SpaceInfos

	deployParams               map[string]application.DeployApplicationParams
	addRemoteApplicationParams state.AddRemoteApplicationParams
	consumeApplicationArgs     params.ConsumeApplicationArgsV5
}

var _ = gc.Suite(&ApplicationSuite{})

// Alternate placing storage instaces in detached, then destroyed
func fakeClassifyDetachedStorage(
	_ storagecommon.VolumeAccess,
	_ storagecommon.FilesystemAccess,
	storage []state.StorageInstance,
) ([]params.Entity, []params.Entity, error) {
	destroyed := make([]params.Entity, 0)
	detached := make([]params.Entity, 0)
	for i, stor := range storage {
		if i%2 == 0 {
			detached = append(detached, params.Entity{stor.StorageTag().String()})
		} else {
			destroyed = append(destroyed, params.Entity{stor.StorageTag().String()})
		}
	}
	return destroyed, detached, nil
}

func fakeSupportedFeaturesGetter(stateenvirons.Model, environs.NewEnvironFunc) (coreassumes.FeatureSet, error) {
	return coreassumes.FeatureSet{}, nil
}

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.modelType = state.ModelTypeIAAS
	s.PatchValue(&application.ClassifyDetachedStorage, fakeClassifyDetachedStorage)
	s.PatchValue(&application.SupportedFeaturesGetter, fakeSupportedFeaturesGetter)
	s.deployParams = make(map[string]application.DeployApplicationParams)

	s.changeAllowed = nil
	s.removeAllowed = nil

	s.allSpaceInfos = network.SpaceInfos{}

	testMac := apitesting.MustNewMacaroon("test")
	s.addRemoteApplicationParams = state.AddRemoteApplicationParams{
		Name:        "hosted-mysql",
		OfferUUID:   "hosted-mysql-uuid",
		URL:         "othermodel.hosted-mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints:   []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider"}},
		Macaroon:    testMac,
	}

	s.consumeApplicationArgs = params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         coretesting.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
			},
			Macaroon: testMac,
		}},
	}
}

func (s *ApplicationSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = mocks.NewMockBackend(ctrl)
	s.backend.EXPECT().ControllerConfig().Return(
		controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, map[string]interface{}{}),
	).AnyTimes()
	s.backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()
	s.backend.EXPECT().AllSpaceInfos().Return(s.allSpaceInfos, nil).AnyTimes()

	s.secrets = mocks.NewMockSecretsStore(ctrl)

	s.storageAccess = mocks.NewMockStorageInterface(ctrl)
	s.storageAccess.EXPECT().VolumeAccess().Return(nil).AnyTimes()
	s.storageAccess.EXPECT().FilesystemAccess().Return(nil).AnyTimes()

	s.blockChecker = mocks.NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed().Return(s.changeAllowed).AnyTimes()
	s.blockChecker.EXPECT().RemoveAllowed().Return(s.removeAllowed).AnyTimes()

	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().ModelTag().Return(coretesting.ModelTag).AnyTimes()
	s.model.EXPECT().Type().Return(s.modelType).MinTimes(1)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig())).AnyTimes()
	s.backend.EXPECT().Model().Return(s.model, nil).AnyTimes()

	s.leadershipReader = mocks.NewMockReader(ctrl)
	s.leadershipReader.EXPECT().Leaders().Return(map[string]string{
		"postgresql": "postgresql/0",
	}, nil).AnyTimes()

	s.storagePoolManager = mocks.NewMockPoolManager(ctrl)

	s.registry = mocks.NewMockProviderRegistry(ctrl)
	s.caasBroker = mocks.NewMockCaasBrokerInterface(ctrl)
	ver := version.MustParse("1.15.0")
	s.caasBroker.EXPECT().Version().Return(&ver, nil).AnyTimes()

	api, err := application.NewAPIBase(
		s.backend,
		s.storageAccess,
		s.authorizer,
		nil,
		nil,
		s.blockChecker,
		s.model,
		s.leadershipReader,
		func(application.Charm) *state.Charm {
			return nil
		},
		func(_ application.ApplicationDeployer, _ application.Model, p application.DeployApplicationParams) (application.Application, error) {
			s.deployParams[p.ApplicationName] = p
			return nil, nil
		},
		s.storagePoolManager,
		s.registry,
		common.NewResources(),
		s.caasBroker,
		func(backendID string) (*secretsprovider.ModelBackendConfigInfo, error) {
			return &secretsprovider.ModelBackendConfigInfo{}, nil
		},
		s.secrets,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *ApplicationSuite) expectCharm(
	ctrl *gomock.Controller, meta *charm.Meta, manifest *charm.Manifest, config *charm.Config,
) *mocks.MockCharm {
	ch := mocks.NewMockCharm(ctrl)
	if manifest != nil {
		ch.EXPECT().Manifest().Return(manifest).AnyTimes()
	}
	if meta != nil {
		ch.EXPECT().Meta().Return(meta).AnyTimes()
	}
	if config != nil {
		ch.EXPECT().Config().Return(config).AnyTimes()
	}
	return ch
}

var defaultCharmConfig = &charm.Config{Options: map[string]charm.Option{
	"stringOption": {Type: "string"},
	"intOption":    {Type: "int", Default: int(123)},
}}

func (s *ApplicationSuite) expectDefaultCharm(ctrl *gomock.Controller) *mocks.MockCharm {
	return s.expectCharm(ctrl, &charm.Meta{Name: "charm-postgresql"}, &charm.Manifest{}, defaultCharmConfig)
}

func (s *ApplicationSuite) expectLxdProfilerCharm(ctrl *gomock.Controller, lxdProfile *charm.LXDProfile) *mockLXDProfilerCharm {
	ch := s.expectDefaultCharm(ctrl)
	lxdProfiler := mocks.NewMockLXDProfiler(ctrl)
	lxdProfiler.EXPECT().LXDProfile().Return(lxdProfile).AnyTimes()
	return &mockLXDProfilerCharm{
		MockCharm:       ch,
		MockLXDProfiler: lxdProfiler,
	}
}

func (s *ApplicationSuite) expectDefaultLxdProfilerCharm(ctrl *gomock.Controller) *mockLXDProfilerCharm {
	return s.expectLxdProfilerCharm(ctrl, &charm.LXDProfile{Config: map[string]string{"security.nested": "false"}})
}

func (s *ApplicationSuite) expectApplication(ctrl *gomock.Controller, name string) *mocks.MockApplication {
	app := mocks.NewMockApplication(ctrl)
	app.EXPECT().Name().Return(name).AnyTimes()
	app.EXPECT().ApplicationTag().Return(names.NewApplicationTag(name)).AnyTimes()
	app.EXPECT().IsPrincipal().Return(true).AnyTimes()
	app.EXPECT().IsExposed().Return(false).AnyTimes()
	app.EXPECT().IsRemote().Return(false).AnyTimes()
	app.EXPECT().Life().Return(state.Alive).AnyTimes()
	app.EXPECT().Constraints().Return(constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"), nil).AnyTimes()
	return app
}

func (s *ApplicationSuite) expectDefaultApplication(ctrl *gomock.Controller) *mocks.MockApplication {
	ch := s.expectDefaultCharm(ctrl)
	return s.expectApplicationWithCharm(ctrl, ch, "postgresql")
}

func (s *ApplicationSuite) expectApplicationWithCharm(ctrl *gomock.Controller, ch application.Charm, name string) *mocks.MockApplication {
	app := s.expectApplication(ctrl, name)
	app.EXPECT().Charm().Return(ch, true, nil).AnyTimes()
	curl := fmt.Sprintf("ch:%s-42", name)
	app.EXPECT().CharmURL().Return(&curl, false).AnyTimes()
	return app
}

func (s *ApplicationSuite) expectUpdateApplicationConfig(c *gc.C, app *mocks.MockApplication) {
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreconfig.NewConfig(map[string]interface{}{
		"juju-external-hostname": "foo",
	}, appCfgSchema, nil)
	c.Assert(err, jc.ErrorIsNil)
	app.EXPECT().UpdateApplicationConfig(appCfg.Attributes(), []string(nil), appCfgSchema, defaults).Return(nil)
}

func (s *ApplicationSuite) TestSetCAASConfigSettings(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig("master", charm.Settings{
		"stringOption": "bar",
	})

	s.expectUpdateApplicationConfig(c, app)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Update settings for the application.
	args := params.ConfigSetArgs{Args: []params.ConfigSet{{
		ApplicationName: "postgresql",
		ConfigYAML:      "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}}}
	results, err := s.api.SetConfigs(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{}})
}

func (s *ApplicationSuite) TestSetCAASConfigSettingsInIAASModelTriggersError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Update settings for the application.
	args := params.ConfigSetArgs{Args: []params.ConfigSet{{
		ApplicationName: "postgresql",
		ConfigYAML:      "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}}}

	results, err := s.api.SetConfigs(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{
		Error: &params.Error{
			Message: "parsing settings for application: unknown option \"juju-external-hostname\"",
		},
	}}, gc.Commentf("expected to get an error when attempting to set CAAS-specific app setting in IAAS model"))
}

func (s *ApplicationSuite) TestSetCharm(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:something-else"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{CharmOrigin: createStateCharmOriginFromURL(curl)}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmEverything(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:something-else"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "foo", "intOption": int64(666)},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg})

	schemaFields, defaults, err := application.AddTrustSchemaAndDefaults(environschema.Fields{}, schema.Defaults{})
	c.Assert(err, jc.ErrorIsNil)
	app.EXPECT().UpdateApplicationConfig(coreconfig.ConfigAttributes{"trust": true}, nil, schemaFields, defaults)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err = s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName:    "postgresql",
		CharmURL:           curl,
		CharmOrigin:        createCharmOriginFromURL(curl),
		ConfigSettings:     map[string]string{"trust": "true", "stringOption": "foo"},
		ConfigSettingsYAML: `postgresql: {"stringOption": "bar", "intOption": 666}`,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmWithBlockRemove(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	s.TestSetCharm(c)
}

func (s *ApplicationSuite) TestSetCharmWithBlockChange(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:something-else",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
	})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestSetCharmRejectCharmStore(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:something-else",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-store", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
	})
	c.Assert(err, gc.ErrorMatches, `"charm-store" not a valid charm origin source`)
}

func (s *ApplicationSuite) TestSetCharmForceUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:something-else"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)

	cfg := state.SetCharmConfig{
		CharmOrigin: createStateCharmOriginFromURL(curl),
		ForceUnits:  true,
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg})

	s.backend.EXPECT().Application("postgresql").Return(app, nil)
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		ForceUnits:      true,
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmWithBlocksAndForceUnits(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	s.changeAllowed = errors.New("change blocked")
	s.TestSetCharmForceUnits(c)
}

func (s *ApplicationSuite) TestSetCharmInvalidApplication(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().Application("badapplication").Return(nil, errors.NotFoundf(`application "badapplication"`))
	curl := "ch:something-else"
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "badapplication",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		ForceBase:       true,
		ForceUnits:      true,
	})
	c.Assert(err, gc.ErrorMatches, `application "badapplication" not found`)
}

func (s *ApplicationSuite) TestSetCharmStorageConstraints(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin: createStateCharmOriginFromURL(curl),
		StorageConstraints: map[string]state.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: 123},
			"d": {Count: 456},
		},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg})

	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		StorageConstraints: map[string]params.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: toUint64Ptr(123)},
			"d": {Count: toUint64Ptr(456)},
		},
		CharmOrigin: createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCAASCharmInvalid(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{Deployment: &charm.Deployment{}}, nil, nil)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "Juju on containers does not support updating deployment info.*")
}

type setCharmConfigMatcher struct {
	c        *gc.C
	expected state.SetCharmConfig
}

func (m setCharmConfigMatcher) Matches(x interface{}) bool {
	req, ok := x.(state.SetCharmConfig)
	if !ok {
		return false
	}
	m.c.Logf("req.CharmOrigin %s", pretty.Sprint(req.CharmOrigin))
	m.c.Logf("m.expected.CharmOrigin %s", pretty.Sprint(m.expected.CharmOrigin))
	m.c.Check(req, gc.DeepEquals, m.expected)
	return true
}

func (setCharmConfigMatcher) String() string {
	return "matches set charm configrequests"
}

func (s *ApplicationSuite) TestSetCharmConfigSettings(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetCharm(state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmDisallowDowngradeFormat(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectCharm(ctrl, &charm.Meta{Name: "charm-postgresql"}, &charm.Manifest{Bases: []charm.Base{{}}}, nil)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, gc.ErrorMatches, "cannot downgrade from v2 charm format to v1")
}

func (s *ApplicationSuite) TestSetCharmUpgradeFormat(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{
		Name: "postgresql",
		Containers: map[string]charm.Container{ // sidecar charm has containers
			"c1": {Resource: "c1-image"},
		},
	}, &charm.Manifest{Bases: []charm.Base{{ // len(bases)>0 means it's a v2 charm
		Name: "ubuntu",
		Channel: charm.Channel{
			Track: "22.04",
			Risk:  "stable",
		},
	}}}, defaultCharmConfig)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	oldCharm := s.expectCharm(ctrl,
		&charm.Meta{
			Name:   "charm-postgresql",
			Series: []string{"kubernetes"},
		},
		&charm.Manifest{},
		&charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	)

	app := s.expectApplicationWithCharm(ctrl, oldCharm, "postgresql")
	cfg := state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		RequireNoUnits: true,
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmConfigSettingsYAML(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetCharm(state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		ConfigSettingsYAML: `
postgresql:
  stringOption: value
`,
	})
	c.Assert(err, jc.ErrorIsNil)
}

var agentTools = tools.Tools{
	Version: version.Binary{
		Number:  version.Number{Major: 2, Minor: 6, Patch: 0},
		Release: "ubuntu",
		Arch:    "x86",
	},
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithNewerAgentVersion(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultLxdProfilerCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	app.EXPECT().SetCharm(state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithOldAgentVersion(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultLxdProfilerCharm(ctrl)
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 5, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, gc.ErrorMatches, "Unable to upgrade LXDProfile charms with the current model version. "+
		"Please run juju upgrade-model to upgrade the current model to match your controller.")
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithEmptyProfile(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectLxdProfilerCharm(ctrl, &charm.LXDProfile{})
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	app.EXPECT().SetCharm(state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmAssumesNotSatisfied(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(
		ctrl,
		&charm.Meta{
			Assumes: &assumes.ExpressionTree{
				Expression: assumes.CompositeExpression{
					ExprType: assumes.AllOfExpression,
					SubExpressions: []assumes.Expression{
						assumes.FeatureExpression{Name: "popcorn"},
					},
				},
			},
		},
		nil,
		&charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	)

	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Try to upgrade the charm
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, gc.ErrorMatches, `(?s)Charm cannot be deployed because:
  - charm requires feature "popcorn" but model does not support it
`)
}

func (s *ApplicationSuite) TestSetCharmAssumesNotSatisfiedWithForce(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(
		ctrl,
		&charm.Meta{
			Assumes: &assumes.ExpressionTree{
				Expression: assumes.CompositeExpression{
					ExprType: assumes.AllOfExpression,
					SubExpressions: []assumes.Expression{
						assumes.FeatureExpression{Name: "popcorn"},
					},
				},
			},
		},
		nil,
		&charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	)

	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetCharm(gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Try to upgrade the charm
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		CharmOrigin:     createCharmOriginFromURL(curl),
		ConfigSettings:  map[string]string{"stringOption": "value"},
		Force:           true,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("expected SetCharm to succeed when --force is set"))
}

func (s *ApplicationSuite) TestDestroyRelation(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	relation := mocks.NewMockRelation(ctrl)
	relation.EXPECT().DestroyWithForce(false, gomock.Any())
	s.backend.EXPECT().InferActiveRelation("a", "b").Return(relation, nil)

	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyRelationNoRelationsFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferActiveRelation("a", "b").Return(nil, errors.New("no relations found"))

	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *ApplicationSuite) TestDestroyRelationRelationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferActiveRelation("a:b", "c:d").Return(nil, errors.NotFoundf(`relation "a:b c:d"`))

	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a:b", "c:d"}})
	c.Assert(err, gc.ErrorMatches, `relation "a:b c:d" not found`)
}

func (s *ApplicationSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	defer s.setup(c).Finish()

	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "remove blocked")
}

func (s *ApplicationSuite) TestDestroyRelationId(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	relation := mocks.NewMockRelation(ctrl)
	relation.EXPECT().DestroyWithForce(false, gomock.Any())
	s.backend.EXPECT().Relation(123).Return(relation, nil)

	err := s.api.DestroyRelation(params.DestroyRelation{RelationId: 123})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyRelationIdRelationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().Relation(123).Return(nil, errors.NotFoundf(`relation "123"`))

	err := s.api.DestroyRelation(params.DestroyRelation{RelationId: 123})
	c.Assert(err, gc.ErrorMatches, `relation "123" not found`)
}

func (s *ApplicationSuite) expectUnit(ctrl *gomock.Controller, name string) *mocks.MockUnit {
	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().Name().Return(name).AnyTimes()
	unit.EXPECT().UnitTag().Return(names.NewUnitTag(name)).AnyTimes()
	unit.EXPECT().Tag().Return(names.NewUnitTag(name)).AnyTimes()
	appName := strings.Split(name, "/")[0]
	machineId := strings.Split(name, "/")[1]
	unit.EXPECT().ApplicationName().Return(appName).AnyTimes()
	unit.EXPECT().AssignedMachineId().Return(machineId, nil).AnyTimes()
	unit.EXPECT().WorkloadVersion().Return("666", nil).AnyTimes()
	unit.EXPECT().Life().Return(state.Alive).AnyTimes()
	return unit
}

func (s *ApplicationSuite) expectStorageAttachment(ctrl *gomock.Controller, storage string) *mocks.MockStorageAttachment {
	sa := mocks.NewMockStorageAttachment(ctrl)
	sa.EXPECT().StorageInstance().Return(names.NewStorageTag(storage))
	return sa
}

func (s *ApplicationSuite) expectStorageInstance(ctrl *gomock.Controller, name string) *mocks.MockStorageInstance {
	storageInstace := mocks.NewMockStorageInstance(ctrl)
	storageInstace.EXPECT().StorageTag().Return(names.NewStorageTag(name)).AnyTimes()
	return storageInstace
}

func (s *ApplicationSuite) expectDefaultStorageAttachments(ctrl *gomock.Controller) {
	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)
	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return([]state.StorageAttachment{}, nil)
}

func (s *ApplicationSuite) assertDefaultDestruction(c *gc.C, results params.DestroyApplicationResults, err error) {
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Info: &params.DestroyApplicationInfo{
			DestroyedUnits: []params.Entity{
				{Tag: "unit-postgresql-0"},
				{Tag: "unit-postgresql-1"},
			},
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	})
}

func (s *ApplicationSuite) TestDestroyApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation().Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	s.backend.EXPECT().ApplyOperation(gomock.AssignableToTypeOf(&state.DestroyApplicationOperation{})).Return(nil)

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
		}},
	})
	s.assertDefaultDestruction(c, results, err)
}

func (s *ApplicationSuite) TestDestroyApplicationWithBlockChange(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	s.TestDestroyApplication(c)
}

func (s *ApplicationSuite) TestDestroyApplicationWithBlockRemove(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	defer s.setup(c).Finish()

	_, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
		}},
	})
	c.Assert(err, gc.ErrorMatches, "remove blocked")
}

type destroyAppMatcher struct {
	c        *gc.C
	expected *state.DestroyApplicationOperation
}

func (m destroyAppMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(*state.DestroyApplicationOperation)
	if !ok {
		return false
	}
	m.c.Assert(obtained.SecretContentDeleter, gc.NotNil)
	obtained.SecretContentDeleter = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m destroyAppMatcher) String() string {
	return pretty.Sprintf("Match the contents of %v", m.expected)
}

func (s *ApplicationSuite) TestForceDestroyApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation().Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	zero := time.Duration(0)

	s.backend.EXPECT().ApplyOperation(destroyAppMatcher{c: c, expected: &state.DestroyApplicationOperation{
		ForcedOperation: state.ForcedOperation{
			Force:   true,
			MaxWait: common.MaxWait(&zero),
		}}}).Return(nil)

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			Force:          true,
			MaxWait:        &zero,
		}},
	})
	s.assertDefaultDestruction(c, results, err)
}

func (s *ApplicationSuite) TestDestroyApplicationDestroyStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation().Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	s.backend.EXPECT().ApplyOperation(destroyAppMatcher{c: c, expected: &state.DestroyApplicationOperation{
		DestroyStorage: true,
	}}).Return(nil)

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			DestroyStorage: true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Info: &params.DestroyApplicationInfo{
			DestroyedUnits: []params.Entity{
				{Tag: "unit-postgresql-0"},
				{Tag: "unit-postgresql-1"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
				{Tag: "storage-pgdata-1"},
			},
		},
	})
}

func (s *ApplicationSuite) TestDestroyApplicationDryRun(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)
	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			DryRun:         true,
		}},
	})
	s.assertDefaultDestruction(c, results, err)
}

func (s *ApplicationSuite) TestDestroyApplicationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().Application("postgresql").Return(nil, errors.NotFoundf(`application "postgresql"`))

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "postgresql" not found`,
		},
	})
}

func (s *ApplicationSuite) expectRemoteApplication(ctrl *gomock.Controller, life state.Life, appStatus status.Status) *mocks.MockRemoteApplication {
	remApp := mocks.NewMockRemoteApplication(ctrl)
	remApp.EXPECT().SourceModel().Return(coretesting.ModelTag).AnyTimes()
	remApp.EXPECT().Life().Return(life).AnyTimes()
	remApp.EXPECT().Status().Return(status.StatusInfo{Status: appStatus}, nil).AnyTimes()
	return remApp
}

func (s *ApplicationSuite) TestDestroyConsumedApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	remApp := s.expectRemoteApplication(ctrl, state.Alive, status.Active)
	remApp.EXPECT().DestroyOperation(false).Return(&state.DestroyRemoteApplicationOperation{})
	s.backend.EXPECT().RemoteApplication("hosted-db2").Return(remApp, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyRemoteApplicationOperation{}).Return(nil)

	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{ApplicationTag: "application-hosted-db2"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{})
}

func (s *ApplicationSuite) TestForceDestroyConsumedApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	force := true
	zero := time.Duration(0)

	remApp := s.expectRemoteApplication(ctrl, state.Alive, status.Active)
	remApp.EXPECT().DestroyOperation(true).Return(&state.DestroyRemoteApplicationOperation{})
	s.backend.EXPECT().RemoteApplication("hosted-db2").Return(remApp, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyRemoteApplicationOperation{ForcedOperation: state.ForcedOperation{
		MaxWait: zero,
	}}).Return(nil)

	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{
			ApplicationTag: "application-hosted-db2",
			Force:          &force,
			MaxWait:        &zero,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{})
}

func (s *ApplicationSuite) TestDestroyConsumedApplicationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().RemoteApplication("hosted-db2").Return(nil, errors.NotFoundf(`saas application "hosted-db2"`))

	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{ApplicationTag: "application-hosted-db2"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `saas application "hosted-db2" not found`,
		},
	})
}

func (s *ApplicationSuite) TestDestroyUnit(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").MinTimes(1).Return(app, nil)

	// unit 0 loop
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{}).Return(nil)

	// unit 1 loop
	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	unit1.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

	s.expectDefaultStorageAttachments(ctrl)

	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{
				UnitTag: "unit-postgresql-0",
			}, {
				UnitTag:        "unit-postgresql-1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, jc.DeepEquals, []params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	}, {
		Info: &params.DestroyUnitInfo{},
	}})
}

func (s *ApplicationSuite) TestDestroyUnitWithChangeBlock(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	s.TestDestroyUnit(c)
}

func (s *ApplicationSuite) TestDestroyUnitWithRemoveBlock(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	defer s.setup(c).Finish()

	_, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: "unit-postgresql-1",
		}},
	})
	c.Assert(err, gc.ErrorMatches, "remove blocked")
}

func (s *ApplicationSuite) TestForceDestroyUnit(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").MinTimes(1).Return(app, nil)

	// unit 0 loop
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	zero := time.Duration(0)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{ForcedOperation: state.ForcedOperation{
		Force:   true,
		MaxWait: common.MaxWait(&zero),
	}}).Return(nil)

	// unit 1 loop
	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	unit1.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

	s.expectDefaultStorageAttachments(ctrl)

	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{
				UnitTag: "unit-postgresql-0",
				Force:   true,
				MaxWait: &zero,
			}, {
				UnitTag:        "unit-postgresql-1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, jc.DeepEquals, []params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	}, {
		Info: &params.DestroyUnitInfo{},
	}})
}

func (s *ApplicationSuite) TestDestroySubordinateUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unit0 := s.expectUnit(ctrl, "subordinate/0")
	unit0.EXPECT().IsPrincipal().Return(false)
	s.backend.EXPECT().Unit("subordinate/0").Return(unit0, nil)

	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: "unit-subordinate-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `unit "subordinate/0" is a subordinate, .*`)
}

func (s *ApplicationSuite) TestDestroyUnitDryRun(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").MinTimes(1).Return(app, nil)

	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)

	s.expectDefaultStorageAttachments(ctrl)

	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{
				UnitTag: "unit-postgresql-0",
				DryRun:  true,
			}, {
				UnitTag: "unit-postgresql-1",
				DryRun:  true,
			},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, jc.DeepEquals, []params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	}, {
		Info: &params.DestroyUnitInfo{},
	}})
}

func (s *ApplicationSuite) TestDeployAttachStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(3)
	s.model.EXPECT().UUID().Return("").AnyTimes()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			AttachStorage:   []string{"storage-foo-0"},
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-1",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        2,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-2",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			AttachStorage:   []string{"volume-baz-0"},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
	c.Assert(results.Results[2].Error, gc.ErrorMatches, `"volume-baz-0" is not a valid volume tag`)
}

func (s *ApplicationSuite) TestDeployCharmOrigin(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(2)
	s.model.EXPECT().UUID().Return("").AnyTimes()

	track := "latest"
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-4",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}, {
			ApplicationName: "bar",
			CharmURL:        "cs:bar-0",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-store",
				Risk:   "stable",
				Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			},
			NumUnits: 1,
		}, {
			ApplicationName: "hub",
			CharmURL:        "hub-7",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-hub",
				Risk:   "stable",
				Track:  &track,
				Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			},
			NumUnits: 1,
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"charm-store" not a valid charm origin source`)
	c.Assert(results.Results[2].Error, gc.IsNil)

	c.Assert(s.deployParams["foo"].CharmOrigin.Source, gc.Equals, corecharm.Source("local"))
	c.Assert(s.deployParams["hub"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-hub"))

	// assert revision is filled in from the charm url
	c.Assert(*s.deployParams["foo"].CharmOrigin.Revision, gc.Equals, 4)
	c.Assert(*s.deployParams["hub"].CharmOrigin.Revision, gc.Equals, 7)
}

func createCharmOriginFromURL(url string) *params.CharmOrigin {
	curl := charm.MustParseURL(url)
	switch curl.Schema {
	case "local":
		return &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}}
	default:
		return &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}}
	}
}

func createStateCharmOriginFromURL(url string) *state.CharmOrigin {
	curl := charm.MustParseURL(url)
	switch curl.Schema {
	case "local":
		return &state.CharmOrigin{Source: "local", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}}
	default:
		return &state.CharmOrigin{Source: "charm-hub", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04"}}
	}
}

func (s *ApplicationSuite) TestApplicationDeployWithStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{Storage: map[string]charm.Storage{
		"data":    {Type: charm.StorageBlock},
		"allecto": {Type: charm.StorageBlock},
	}}, nil, &charm.Config{})
	curl := "ch:utopic/storage-block-10"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)
	s.model.EXPECT().UUID().Return("")

	storageConstraints := map[string]storage.Constraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "modelscoped-block",
		},
	}
	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
		Storage:         storageConstraints,
	}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Storage, gc.DeepEquals, storageConstraints)
}

func (s *ApplicationSuite) TestApplicationDeployDefaultFilesystemStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{Storage: map[string]charm.Storage{
		"data": {Type: charm.StorageFilesystem, ReadOnly: true},
	}}, nil, &charm.Config{})
	curl := "ch:utopic/storage-filesystem-1"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
	}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestApplicationDeployPlacement(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:precise/dummy-42"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)
	machine.EXPECT().IsParentLockedForSeriesUpgrade().Return(false, nil)
	s.backend.EXPECT().Machine("valid").Return(machine, nil)
	s.model.EXPECT().UUID().Return("")

	placement := []*instance.Placement{
		{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "valid"},
	}
	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
		Placement:       placement,
	}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Placement, gc.DeepEquals, placement)
}

func (s *ApplicationSuite) TestApplicationDeployPlacementModelUUIDSubstitute(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:precise/dummy-42"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)
	s.model.EXPECT().UUID().Return("deadbeef-0bad-400d-8000-4b1d0d06f00d")

	placement := []*instance.Placement{
		{Scope: "model-uuid", Directive: "0"},
	}
	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
		Placement:       placement,
	}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Placement, gc.DeepEquals, []*instance.Placement{
		{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "0"},
	})
}

func validCharmOriginForTest() *params.CharmOrigin {
	return &params.CharmOrigin{Source: "charm-hub"}
}

func (s *ApplicationSuite) TestApplicationDeployWithPlacementLockedError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(true, nil).MinTimes(1)
	s.backend.EXPECT().Machine("0").Return(machine, nil).MinTimes(1)
	containerMachine := mocks.NewMockMachine(ctrl)
	containerMachine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil).MinTimes(1)
	containerMachine.EXPECT().IsParentLockedForSeriesUpgrade().Return(true, nil).MinTimes(1)
	s.backend.EXPECT().Machine("0/lxd/0").Return(containerMachine, nil).MinTimes(1)
	s.model.EXPECT().UUID().Return("").AnyTimes()

	curl := "ch:precise/dummy-42"
	args := []params.ApplicationDeploy{{
		ApplicationName: "machine-placement",
		CharmURL:        curl,
		CharmOrigin:     validCharmOriginForTest(),
		Placement:       []*instance.Placement{{Scope: "#", Directive: "0"}},
	}, {
		ApplicationName: "container-placement",
		CharmURL:        curl,
		CharmOrigin:     validCharmOriginForTest(),
		Placement:       []*instance.Placement{{Scope: "lxd", Directive: "0"}},
	}, {
		ApplicationName: "container-placement-locked-parent",
		CharmURL:        curl,
		CharmOrigin:     validCharmOriginForTest(),
		Placement:       []*instance.Placement{{Scope: "#", Directive: "0/lxd/0"}},
	}}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: args,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error.Error(), gc.Matches, ".*: machine is locked for series upgrade")
	c.Assert(results.Results[1].Error.Error(), gc.Matches, ".*: machine is locked for series upgrade")
	c.Assert(results.Results[2].Error.Error(), gc.Matches, ".*: parent machine is locked for series upgrade")
}

func (s *ApplicationSuite) TestApplicationDeployFailCharmOrigin(c *gc.C) {
	originId := createCharmOriginFromURL("ch:jammy/test-42")
	originId.ID = "testingID"
	originHash := createCharmOriginFromURL("ch:jammy/test-42")
	originHash.Hash = "testing-hash"

	args := []params.ApplicationDeploy{{
		CharmOrigin: originId,
	}, {
		CharmOrigin: originHash,
	}}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: args,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error.Code, gc.Equals, "bad request")
	c.Assert(results.Results[1].Error.Code, gc.Equals, "bad request")
}

func (s *ApplicationSuite) TestApplicationDeploymentRemovesPendingResourcesOnFailure(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	resources := map[string]string{"dummy": "pending-id"}
	resourcesManager := mocks.NewMockResources(ctrl)
	resourcesManager.EXPECT().RemovePendingAppResources("my-app", resources).Return(nil)
	s.backend.EXPECT().Resources().Return(resourcesManager)

	rev := 8
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			// CharmURL is missing & revision included to ensure deployApplication
			// fails sufficiently far through execution so that we can assert pending
			// app resources are removed
			ApplicationName: "my-app",
			CharmOrigin: &params.CharmOrigin{
				Source:   "charm-hub",
				Revision: &rev,
			},
			Resources: resources,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *ApplicationSuite) TestApplicationDeploymentTrust(c *gc.C) {
	// This test should fail if the configuration parsing does not
	// understand the "trust" configuration parameter
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:precise/dummy-42"
	s.backend.EXPECT().Charm(curl).Return(ch, nil).MinTimes(1)
	s.model.EXPECT().UUID().Return("")

	withTrust := map[string]string{"trust": "true"}
	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
		Config:          withTrust,
	}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].ApplicationConfig.Attributes().GetBool("trust", false), gc.Equals, true)
}

func (s *ApplicationSuite) TestClientApplicationsDeployWithBindings(c *gc.C) {
	s.allSpaceInfos = network.SpaceInfos{{ID: "42", Name: "a-space"}}
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:focal/riak-42"
	s.backend.EXPECT().Charm(curl).Return(ch, nil).MinTimes(1)
	s.model.EXPECT().UUID().Return("").AnyTimes()

	args := []params.ApplicationDeploy{{
		ApplicationName: "old",
		CharmURL:        curl,
		CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "focal"}},
		NumUnits:        1,
		EndpointBindings: map[string]string{
			"endpoint": "a-space",
			"ring":     "",
			"admin":    "",
		},
	}, {
		ApplicationName: "regular",
		CharmURL:        curl,
		CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "focal"}},
		NumUnits:        1,
		EndpointBindings: map[string]string{
			"endpoint": "42",
		},
	}}
	results, err := s.api.Deploy(params.ApplicationsDeploy{
		Applications: args,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)

	c.Assert(s.deployParams["old"].EndpointBindings, gc.DeepEquals, map[string]string{
		"endpoint": "42",
		"ring":     network.AlphaSpaceId,
		"admin":    network.AlphaSpaceId,
	})
	c.Assert(s.deployParams["regular"].EndpointBindings, gc.DeepEquals, map[string]string{
		"endpoint": "42",
	})
}

func (s *ApplicationSuite) expectDefaultK8sModelConfig() {
	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"operator-storage": "k8s-operator-storage",
		"workload-storage": "k8s-storage",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).MinTimes(1)
}

func (s *ApplicationSuite) TestDeployMinDeploymentVersionTooHigh(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{
		Deployment: &charm.Deployment{
			MinVersion: "1.99.0",
		},
	}, &charm.Manifest{}, &charm.Config{})
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	s.expectDefaultK8sModelConfig()
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any()).Return(nil)
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-annotations": "a=b c="},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(
		results.Results[0].Error, gc.ErrorMatches,
		regexp.QuoteMeta(`charm requires a minimum k8s version of 1.99.0 but the cluster only runs version 1.15.0`),
	)
}

func (s *ApplicationSuite) TestDeployCAASModel(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl,
		&charm.Meta{
			// To ensure we don't require k8s operator storage
			MinJujuVersion: version.Number{Major: 2, Minor: 8, Patch: 1},
		},
		&charm.Manifest{},
		&charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(4)
	s.model.EXPECT().UUID().Return("").AnyTimes()
	s.expectDefaultK8sModelConfig()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-annotations": "a=b c="},
			ConfigYAML:      "foo:\n  stringOption: fred\n  kubernetes-service-type: loadbalancer",
		}, {
			ApplicationName: "foobar",
			CharmURL:        "local:foobar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-type": "cluster", "intOption": "2"},
			ConfigYAML:      "foobar:\n  intOption: 1\n  kubernetes-service-type: loadbalancer\n  kubernetes-ingress-ssl-redirect: true",
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Placement:       []*instance.Placement{{}, {}},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 4)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.IsNil)
	c.Assert(results.Results[3].Error, gc.ErrorMatches, "only 1 placement directive is supported for container models, got 2")

	c.Assert(s.deployParams["foo"].ApplicationConfig.Attributes()["kubernetes-service-type"], gc.Equals, "loadbalancer")
	// Check attach storage
	c.Assert(s.deployParams["bar"].AttachStorage, jc.DeepEquals, []names.StorageTag{names.NewStorageTag("bar/0")})
	// Check parsing of k8s service annotations.
	c.Assert(s.deployParams["foo"].ApplicationConfig.Attributes()["kubernetes-service-annotations"], jc.DeepEquals, map[string]string{"a": "b", "c": ""})
	c.Assert(s.deployParams["foobar"].ApplicationConfig.Attributes()["kubernetes-service-type"], gc.Equals, "cluster")
	c.Assert(s.deployParams["foobar"].ApplicationConfig.Attributes()["kubernetes-ingress-ssl-redirect"], gc.Equals, true)
	c.Assert(s.deployParams["foobar"].CharmConfig, jc.DeepEquals, charm.Settings{"intOption": int64(2)})
}

func (s *ApplicationSuite) TestDeployCAASInvalidServiceType(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.model.EXPECT().UUID().Return("")

	curl := "local:foo-0"
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-type": "ClusterIP", "intOption": "2"},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.OneError(), gc.ErrorMatches, `service type "ClusterIP" not valid`)
}

func (s *ApplicationSuite) TestDeployCAASBlockStorageRejected(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{
		Storage: map[string]charm.Storage{"block": {Name: "block", Type: charm.StorageBlock}},
	}, &charm.Manifest{}, &charm.Config{})
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin: &params.CharmOrigin{
				Source:       "local",
				Architecture: "amd64",
				Base: params.Base{
					Name:    "ubuntu",
					Channel: "22.04/stable",
				}},
			NumUnits: 1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.OneError(), gc.ErrorMatches, `block storage "block" is not supported for container charms`)
}

func (s *ApplicationSuite) TestDeployCAASModelNoOperatorStorage(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).MinTimes(1)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin: &params.CharmOrigin{
				Source:       "local",
				Architecture: "amd64",
				Base: params.Base{
					Name:    "ubuntu",
					Channel: "22.04/stable",
				},
			},
			NumUnits: 1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `deploying this Kubernetes application requires a suitable storage class.*`)
}

func (s *ApplicationSuite) TestDeployCAASModelCharmNeedsNoOperatorStorage(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.PatchValue(&jujuversion.Current, version.MustParse("2.8-beta1"))

	ch := s.expectCharm(ctrl, &charm.Meta{
		MinJujuVersion: version.MustParse("2.8.0"),
	}, &charm.Manifest{}, &charm.Config{})
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).MinTimes(1)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelSidecarCharmNeedsNoOperatorStorage(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{}, &charm.Manifest{Bases: []charm.Base{{}}}, &charm.Config{})
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).MinTimes(1)
	s.model.EXPECT().UUID().Return("").AnyTimes()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultOperatorStorageClass(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.expectDefaultK8sModelConfig()
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any()).Return(nil)
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(nil, errors.NotFoundf("pool"))
	s.registry.EXPECT().StorageProvider(storage.ProviderType("k8s-operator-storage")).Return(nil, errors.NotFoundf(`provider type "k8s-operator-storage"`))
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelWrongOperatorStorageType(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.expectDefaultK8sModelConfig()
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		provider.RootfsProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `the "k8s-operator-storage" storage pool requires a provider type of "kubernetes", not "rootfs"`)
}

func (s *ApplicationSuite) TestDeployCAASModelInvalidStorage(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.expectDefaultK8sModelConfig()
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any()).Return(errors.NotFoundf("storage class"))
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `storage class not found`)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultStorageClass(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.expectDefaultK8sModelConfig()
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.storagePoolManager.EXPECT().Get("k8s-storage").Return(storage.NewConfig(
		"k8s-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any()).Return(nil)
	s.model.EXPECT().UUID().Return("")

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestAddUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	newUnit := s.expectUnit(ctrl, "postgresql/99")
	newUnit.EXPECT().AssignWithPolicy(state.AssignCleanEmpty)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AddUnit(state.AddUnitParams{AttachStorage: []names.StorageTag{}}).Return(newUnit, nil)
	s.backend.EXPECT().Application("postgresql").AnyTimes().Return(app, nil)

	results, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.AddApplicationUnitsResults{
		Units: []string{"postgresql/99"},
	})
}

func (s *ApplicationSuite) TestAddUnitsCAASModel(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, gc.ErrorMatches, "adding units to a container-based model not supported")
}

func (s *ApplicationSuite) TestAddUnitsAttachStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	newUnit := s.expectUnit(ctrl, "postgresql/99")
	newUnit.EXPECT().AssignWithPolicy(state.AssignCleanEmpty)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AddUnit(state.AddUnitParams{
		AttachStorage: []names.StorageTag{names.NewStorageTag("pgdata/0")},
	}).Return(newUnit, nil)
	s.backend.EXPECT().Application("postgresql").AnyTimes().Return(app, nil)

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
		AttachStorage:   []string{"storage-pgdata-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	currentCh := s.expectCharm(ctrl, &charm.Meta{Name: "charm-foo"}, &charm.Manifest{Bases: []charm.Base{{}}}, nil)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "foo")
	s.backend.EXPECT().Application("foo").AnyTimes().Return(app, nil)

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
		AttachStorage:   []string{"storage-foo-0"},
	})
	c.Assert(err, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageInvalidStorageTag(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	currentCh := s.expectCharm(ctrl, &charm.Meta{Name: "charm-foo"}, &charm.Manifest{Bases: []charm.Base{{}}}, nil)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "foo")
	s.backend.EXPECT().Application("foo").AnyTimes().Return(app, nil)

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"volume-0"},
	})
	c.Assert(err, gc.ErrorMatches, `"volume-0" is not a valid storage tag`)
}

func (s *ApplicationSuite) TestDestroyUnitsCAASModel(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	_, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: "unit-postgresql-0"},
			{
				UnitTag:        "unit-postgresql-1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, "removing units on a non-container model not supported")
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModel(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetScale(5, int64(0), true).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{{
			Info: &params.ScaleApplicationInfo{Scale: 5},
		}},
	})
}

func (s *ApplicationSuite) TestScaleApplicationsNotAllowedForOperator(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{
		Deployment: &charm.Deployment{
			DeploymentMode: charm.ModeOperator,
		},
	}, nil, nil)
	app := s.expectApplicationWithCharm(ctrl, ch, "postgresql")
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	}
	result, err := s.api.ScaleApplications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	msg := strings.Replace(result.Results[0].Error.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, `scale an "operator" application not supported`)
}

func (s *ApplicationSuite) TestScaleApplicationsNotAllowedForDaemonSet(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{
		Deployment: &charm.Deployment{
			DeploymentType: charm.DeploymentDaemon,
		},
	}, nil, nil)
	app := s.expectApplicationWithCharm(ctrl, ch, "postgresql")
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	}
	result, err := s.api.ScaleApplications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	msg := strings.Replace(result.Results[0].Error.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, `scale a "daemon" application not supported`)
}

func (s *ApplicationSuite) TestScaleApplicationsBlocked(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleChange(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().ChangeScale(5).Return(7, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			ScaleChange:    5,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{{
			Info: &params.ScaleApplicationInfo{Scale: 7},
		}},
	})
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleArgCheckScaleAndScaleChange(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
			ScaleChange:    5,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "requesting both scale and scale-change not valid")
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleArgCheckInvalidScale(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          -1,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "scale < 0 not valid")
}

func (s *ApplicationSuite) TestScaleApplicationsIAASModel(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}}})
	c.Assert(err, gc.ErrorMatches, "scaling applications on a non-container model not supported")
}

func (s *ApplicationSuite) expectRelation(ctrl *gomock.Controller, name string, suspended bool) *mocks.MockRelation {
	rel := mocks.NewMockRelation(ctrl)
	rel.EXPECT().Tag().Return(names.NewRelationTag(name)).AnyTimes()
	rel.EXPECT().Suspended().Return(suspended).AnyTimes()

	ep0 := strings.Split(name, " ")[0]
	appName0 := strings.Split(ep0, ":")[0]
	epName0 := strings.Split(ep0, ":")[1]
	ep1 := strings.Split(name, " ")[1]
	appName1 := strings.Split(ep1, ":")[0]
	epName1 := strings.Split(ep1, ":")[1]

	rel.EXPECT().Endpoint(appName0).Return(state.Endpoint{
		ApplicationName: appName0,
		Relation:        charm.Relation{Name: epName0},
	}, nil).AnyTimes()
	rel.EXPECT().RelatedEndpoints(appName0).Return([]state.Endpoint{{
		ApplicationName: appName1,
		Relation:        charm.Relation{Name: epName1},
	}}, nil).AnyTimes()

	rel.EXPECT().Endpoint(appName1).Return(state.Endpoint{
		ApplicationName: appName1,
		Relation:        charm.Relation{Name: epName1},
	}, nil).AnyTimes()
	rel.EXPECT().RelatedEndpoints(appName1).Return([]state.Endpoint{{
		ApplicationName: appName0,
		Relation:        charm.Relation{Name: epName0},
	}}, nil).AnyTimes()

	rel.EXPECT().ApplicationSettings(appName0).Return(map[string]interface{}{"app-" + appName0: "setting"}, nil).AnyTimes()
	rel.EXPECT().ApplicationSettings(appName1).Return(map[string]interface{}{"app-" + appName1: "setting"}, nil).AnyTimes()

	return rel
}

func (s *ApplicationSuite) TestSetRelationSuspended(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rel := s.expectRelation(ctrl, "wordpress:db mysql:db", false)
	rel.EXPECT().SetSuspended(true, "message").Return(nil)
	rel.EXPECT().SetStatus(status.StatusInfo{
		Status:  status.Suspending,
		Message: "message",
	}).Return(nil)
	s.backend.EXPECT().Relation(123).Return(rel, nil)
	s.backend.EXPECT().OfferConnectionForRelation("wordpress:db mysql:db").Return(nil, nil)

	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
			Message:    "message",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestSetRelationSuspendedNoOp(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rel := s.expectRelation(ctrl, "wordpress:db mysql:db", true)
	s.backend.EXPECT().Relation(123).Return(rel, nil)

	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestSetRelationSuspendedFalse(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rel := s.expectRelation(ctrl, "wordpress:db mysql:db", true)
	rel.EXPECT().SetSuspended(false, "").Return(nil)
	rel.EXPECT().SetStatus(status.StatusInfo{Status: status.Joining}).Return(nil)
	s.backend.EXPECT().Relation(123).Return(rel, nil)
	s.backend.EXPECT().OfferConnectionForRelation("wordpress:db mysql:db").Return(nil, nil)

	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestSetRelationSuspendedPermission(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rel := s.expectRelation(ctrl, "wordpress:db mysql:db", true)
	s.backend.EXPECT().Relation(123).Return(rel, nil)

	offerConn := mocks.NewMockOfferConnection(ctrl)
	offerConn.EXPECT().OfferUUID().Return("offer-uuid")
	offerConn.EXPECT().UserName().Return("fred")
	s.backend.EXPECT().OfferConnectionForRelation("wordpress:db mysql:db").Return(offerConn, nil)

	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestSetNonOfferRelationStatus(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rel := s.expectRelation(ctrl, "mediawiki:db mysql:db", false)
	s.backend.EXPECT().Relation(123).Return(rel, nil)
	s.backend.EXPECT().OfferConnectionForRelation("mediawiki:db mysql:db").Return(nil, errors.NotFoundf(""))

	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, `cannot set suspend status for "mediawiki:db mysql:db" which is not associated with an offer`)
}

func (s *ApplicationSuite) TestBlockSetRelationSuspended(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestSetRelationSuspendedPermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	defer s.setup(c).Finish()

	_, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestConsumeIdempotent(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	remApp := s.expectRemoteApplication(ctrl, state.Alive, status.Active)
	remApp.EXPECT().Endpoints().Return([]state.Endpoint{{
		ApplicationName: "hosted-mysql",
		Relation:        charm.Relation{Name: "database", Interface: "mysql", Role: "provider"},
	}}, nil)

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(nil, errors.NotFoundf(`saas application "hosted-mysql"`))
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(remApp, nil)
	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(remApp, nil)

	for i := 0; i < 2; i++ {
		results, err := s.api.Consume(s.consumeApplicationArgs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.OneError(), gc.IsNil)
	}
}

func (s *ApplicationSuite) TestConsumeFromExternalController(c *gc.C) {
	defer s.setup(c).Finish()

	controllerUUID := utils.MustNewUUID().String()

	s.addRemoteApplicationParams.ExternalControllerUUID = controllerUUID

	s.consumeApplicationArgs.Args[0].ControllerInfo = &params.ExternalControllerInfo{
		ControllerTag: names.NewControllerTag(controllerUUID).String(),
		Alias:         "controller-alias",
		CACert:        coretesting.CACert,
		Addrs:         []string{"192.168.1.1:1234"},
	}

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(nil, errors.NotFoundf(`saas application "hosted-mysql"`))
	s.backend.EXPECT().SaveController(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(controllerUUID),
		Alias:         "controller-alias",
		CACert:        coretesting.CACert,
		Addrs:         []string{"192.168.1.1:1234"},
	}, coretesting.ModelTag.Id()).Return(nil, nil)
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(s.consumeApplicationArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeFromSameController(c *gc.C) {
	defer s.setup(c).Finish()

	s.consumeApplicationArgs.Args[0].ControllerInfo = &params.ExternalControllerInfo{
		ControllerTag: coretesting.ControllerTag.String(),
		Alias:         "controller-alias",
		CACert:        coretesting.CACert,
		Addrs:         []string{"192.168.1.1:1234"},
	}

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(nil, errors.NotFoundf(`saas application "hosted-mysql"`))
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(s.consumeApplicationArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeIncludesSpaceInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.addRemoteApplicationParams.Name = "beirut"

	s.consumeApplicationArgs.Args[0].ApplicationAlias = "beirut"

	s.backend.EXPECT().RemoteApplication("beirut").Return(nil, errors.NotFoundf(`saas application "beirut"`))
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(s.consumeApplicationArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeRemoteAppExistsDifferentSourceModel(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(s.expectRemoteApplication(ctrl, state.Alive, status.Active), nil)

	s.consumeApplicationArgs.Args[0].ApplicationOfferDetailsV5.SourceModelTag = names.NewModelTag(utils.MustNewUUID().String()).String()

	results, err := s.api.Consume(params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(utils.MustNewUUID().String()).String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, `saas application called "hosted-mysql" from a different model already exists`)
}

func (s *ApplicationSuite) TestConsumeRemoteAppDying(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(s.expectRemoteApplication(ctrl, state.Dying, status.Active), nil)

	results, err := s.api.Consume(s.consumeApplicationArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, `saas application called "hosted-mysql" exists but is terminating`)
}

func (s *ApplicationSuite) TestConsumeRemoteAppTerminated(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	remApp := s.expectRemoteApplication(ctrl, state.Alive, status.Terminated)
	remApp.EXPECT().DestroyOperation(true).Return(&state.DestroyRemoteApplicationOperation{})
	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(remApp, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyRemoteApplicationOperation{}).Return(nil)
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(s.consumeApplicationArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestApplicationUpdateBaseNoParams(c *gc.C) {
	defer s.setup(c).Finish()

	results, err := s.api.UpdateApplicationBase(
		params.UpdateChannelArgs{
			Args: []params.UpdateChannelArg{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{}})
}

func (s *ApplicationSuite) TestApplicationUpdateBasePermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	defer s.setup(c).Finish()

	_, err := s.api.UpdateApplicationBase(
		params.UpdateChannelArgs{
			Args: []params.UpdateChannelArg{{
				Entity:  params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
				Channel: "22.04",
			}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestRemoteRelationBadCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferEndpoints("wordpress", "hosted-mysql:nope").Return([]state.Endpoint{{
		ApplicationName: "wordpress",
	}, {
		ApplicationName: "hosted-mysql",
	}}, nil)
	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"bad.cidr"}})
	c.Assert(err, gc.ErrorMatches, `invalid CIDR address: bad.cidr`)
}

func (s *ApplicationSuite) TestNonRemoteRelationCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferEndpoints("wordpress", "mysql").Return([]state.Endpoint{{
		ApplicationName: "wordpress",
	}, {
		ApplicationName: "mysql",
	}}, nil)
	s.backend.EXPECT().RemoteApplication("wordpress").Return(nil, errors.NotFound)
	s.backend.EXPECT().RemoteApplication("mysql").Return(nil, errors.NotFound)
	endpoints := []string{"wordpress", "mysql"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"10.10.0.0/16"}})
	c.Assert(err, gc.ErrorMatches, `integration via subnets for non cross model relations not supported`)
}

func (s *ApplicationSuite) TestRemoteRelationDisAllowedCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferEndpoints("wordpress", "hosted-mysql:nope").Return([]state.Endpoint{{
		ApplicationName: "wordpress",
	}, {
		ApplicationName: "hosted-mysql",
	}}, nil)
	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"0.0.0.0/0"}})
	c.Assert(err, gc.ErrorMatches, `CIDR "0.0.0.0/0" not allowed`)
}

func (s *ApplicationSuite) TestSetConfigBranch(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig("new-branch", charm.Settings{"stringOption": "stringVal"})
	s.expectUpdateApplicationConfig(c, app)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	gen := mocks.NewMockGeneration(ctrl)
	gen.EXPECT().AssignApplication("postgresql")
	s.backend.EXPECT().Branch("new-branch").Return(gen, nil)

	result, err := s.api.SetConfigs(params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "foo",
				"stringOption":           "stringVal",
			},
			Generation: "new-branch",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetEmptyConfigMasterBranch(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig("master", charm.Settings{"stringOption": ""})
	s.expectUpdateApplicationConfig(c, app)

	s.backend.EXPECT().Application("postgresql").Return(app, nil)
	result, err := s.api.SetConfigs(params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "foo",
				"stringOption":           "",
			},
			Generation: "master",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestUnsetApplicationConfig(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	schema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	schema, defaults, err = application.AddTrustSchemaAndDefaults(schema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateApplicationConfig(coreconfig.ConfigAttributes(nil), []string{"juju-external-hostname"}, schema, defaults)
	app.EXPECT().UpdateCharmConfig("new-branch", charm.Settings{"stringVal": nil})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	result, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "postgresql",
			Options:         []string{"juju-external-hostname", "stringVal"},
			BranchName:      "new-branch",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestBlockUnsetApplicationConfig(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestUnsetApplicationConfigPermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	_, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "postgresql",
			Options:         []string{"option"},
		}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestResolveUnitErrors(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().Resolve(true).Return(nil)
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().Resolve(true).Return(nil)
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)

	entities := []params.Entity{{Tag: "unit-postgresql-0"}, {Tag: "unit-postgresql-1"}}
	p := params.UnitsResolved{
		Retry: true,
		Tags: params.Entities{
			Entities: entities,
		},
	}
	result, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *ApplicationSuite) TestResolveUnitErrorsAll(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().Resolve(true).Return(nil)
	s.backend.EXPECT().UnitsInError().Return([]application.Unit{unit0}, nil)

	p := params.UnitsResolved{
		All:   true,
		Retry: true,
	}
	_, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestBlockResolveUnitErrors(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.ResolveUnitErrors(params.UnitsResolved{})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestResolveUnitErrorsPermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	defer s.setup(c).Finish()

	entities := []params.Entity{{Tag: "unit-postgresql-0"}}
	p := params.UnitsResolved{
		Retry: true,
		Tags: params.Entities{
			Entities: entities,
		},
	}
	_, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestCAASExposeWithoutHostname(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.Expose(params.ApplicationExpose{
		ApplicationName: "postgresql",
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot expose a container application without a "juju-external-hostname" value set, run\n`+
			`juju config postgresql juju-external-hostname=<value>`)
}

func (s *ApplicationSuite) TestCAASExposeWithHostname(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{"juju-external-hostname": "exthost"}, nil)
	app.EXPECT().MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR}},
	}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.Expose(params.ApplicationExpose{
		ApplicationName: "postgresql",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestApplicationsInfoOne(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Channel:  &state.Channel{Track: "2.0", Risk: "candidate"},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"},
	}).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

	bindings := mocks.NewMockBindings(ctrl)
	bindings.EXPECT().MapWithSpaceNames(gomock.Any()).Return(map[string]string{"juju-info": "myspace"}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(bindings, nil).MinTimes(1)
	app.EXPECT().ExposedEndpoints().Return(nil).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-postgresql",
		Charm:       "charm-postgresql",
		Base:        params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		Channel:     "2.0/candidate",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		Life:        state.Alive.String(),
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
	})
}

func (s *ApplicationSuite) TestApplicationsInfoOneWithExposedEndpoints(c *gc.C) {
	s.allSpaceInfos = network.SpaceInfos{{ID: "42", Name: "non-euclidean-geometry"}}
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"},
	}).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

	bindings := mocks.NewMockBindings(ctrl)
	bindings.EXPECT().MapWithSpaceNames(gomock.Any()).Return(map[string]string{"juju-info": "myspace"}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(bindings, nil).MinTimes(1)
	app.EXPECT().ExposedEndpoints().Return(map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{"42"},
			ExposeToCIDRs:    []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	}).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-postgresql",
		Charm:       "charm-postgresql",
		Base:        params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		Life:        state.Alive.String(),
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
		ExposedEndpoints: map[string]params.ExposedEndpoint{
			"server": {
				ExposeToSpaces: []string{"non-euclidean-geometry"},
				ExposeToCIDRs:  []string{"10.0.0.0/24", "192.168.0.0/24"},
			},
		},
	})
}

func (s *ApplicationSuite) TestApplicationsInfoDetailsErr(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmConfig("master").Return(nil, errors.Errorf("boom"))
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestApplicationsInfoBindingsErr(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(nil, errors.Errorf("boom")).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestApplicationsInfoMany(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	// postgresql
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"},
	}).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

	bindings := mocks.NewMockBindings(ctrl)
	bindings.EXPECT().MapWithSpaceNames(gomock.Any()).Return(map[string]string{"juju-info": "myspace"}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(bindings, nil).MinTimes(1)
	app.EXPECT().ExposedEndpoints().Return(map[string]state.ExposedEndpoint{}).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	// wordpress
	s.backend.EXPECT().Application("wordpress").Return(nil, errors.NotFoundf(`application "wordpress"`))

	entities := []params.Entity{{Tag: "application-postgresql"}, {Tag: "application-wordpress"}, {Tag: "unit-postgresql-0"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-postgresql",
		Charm:       "charm-postgresql",
		Base:        params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		Life:        state.Alive.String(),
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
	})
	c.Assert(result.Results[1].Error, gc.ErrorMatches, `application "wordpress" not found`)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"unit-postgresql-0" is not a valid application tag`)
}

func (s *ApplicationSuite) TestApplicationMergeBindingsErr(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().MergeBindings(gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	req := params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{
			{
				ApplicationTag: "application-postgresql",
			},
		},
	}
	result, err := s.api.MergeBindings(req)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(req.Args))
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) expectCloudContainer(ctrl *gomock.Controller) *mocks.MockCloudContainer {
	cloudContainer := mocks.NewMockCloudContainer(ctrl)
	cloudContainer.EXPECT().Address().Return(&network.SpaceAddress{
		MachineAddress: network.MachineAddress{Value: "192.168.1.1"},
	}).AnyTimes()
	cloudContainer.EXPECT().ProviderId().Return("provider-id").AnyTimes()
	return cloudContainer
}

func (s *ApplicationSuite) expectUnitWithCloudContainer(ctrl *gomock.Controller, cc state.CloudContainer, name string) *mocks.MockUnit {
	unit := s.expectUnit(ctrl, name)
	unit.EXPECT().ContainerInfo().Return(cc, nil)
	return unit
}

func (s *ApplicationSuite) expectMachineUnitPortRange(ctrl *gomock.Controller, unitName, portRange string) *mocks.MockMachinePortRanges {
	unitPortRanges := mocks.NewMockUnitPortRanges(ctrl)
	unitPortRanges.EXPECT().UniquePortRanges().Return([]network.PortRange{network.MustParsePortRange(portRange)})
	machinePortRange := mocks.NewMockMachinePortRanges(ctrl)
	machinePortRange.EXPECT().ForUnit(unitName).Return(unitPortRanges)
	return machinePortRange
}

func (s *ApplicationSuite) expectRelationUnit(ctrl *gomock.Controller, name string) *mocks.MockRelationUnit {
	relUnit := mocks.NewMockRelationUnit(ctrl)
	relUnit.EXPECT().UnitName().Return(name).AnyTimes()
	relUnit.EXPECT().InScope().Return(true, nil).AnyTimes()
	relUnit.EXPECT().Settings().Return(map[string]interface{}{name: name + "-setting"}, nil).AnyTimes()
	return relUnit
}

func (s *ApplicationSuite) expectMachineWithIP(ctrl *gomock.Controller, publicAddress string) *mocks.MockMachine {
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().PublicAddress().Return(network.SpaceAddress{MachineAddress: network.MachineAddress{Value: publicAddress}}, nil).MinTimes(1)
	return machine
}

func (s *ApplicationSuite) TestUnitsInfo(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unit := s.expectUnitWithCloudContainer(ctrl, s.expectCloudContainer(ctrl), "postgresql/0")
	s.backend.EXPECT().Unit("postgresql/0").Return(unit, nil)

	s.backend.EXPECT().Unit("mysql/0").Return(nil, errors.NotFoundf(`unit "mysql/0"`))

	app := s.expectDefaultApplication(ctrl)

	rel := s.expectRelation(ctrl, "postgresql:db gitlab:server", false)
	rel.EXPECT().Id().Return(101)
	rel.EXPECT().AllRemoteUnits("gitlab").Return([]application.RelationUnit{s.expectRelationUnit(ctrl, "gitlab/2")}, nil)
	app.EXPECT().Relations().Return([]application.Relation{rel}, nil)

	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.backend.EXPECT().Machine("0").Return(s.expectMachineWithIP(ctrl, "10.0.0.1"), nil)

	s.model.EXPECT().OpenedPortRangesForMachine("0").Return(s.expectMachineUnitPortRange(ctrl, "postgresql/0", "100-102/tcp"), nil)

	// gitlab exists is remote in this scenerios so return a not found error
	s.backend.EXPECT().Application("gitlab").Return(nil, errors.NotFoundf(`application "gitlab"`))

	entities := []params.Entity{{Tag: "unit-postgresql-0"}, {Tag: "unit-mysql-0"}}
	result, err := s.api.UnitsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.UnitResult{
		Tag:             "unit-postgresql-0",
		WorkloadVersion: "666",
		Machine:         "0",
		OpenedPorts:     []string{"100-102/tcp"},
		PublicAddress:   "10.0.0.1",
		Charm:           "ch:postgresql-42",
		Leader:          true,
		Life:            state.Alive.String(),
		RelationData: []params.EndpointRelationData{{
			RelationId:      101,
			Endpoint:        "db",
			CrossModel:      true,
			RelatedEndpoint: "server",
			ApplicationData: map[string]interface{}{"app-gitlab": "setting"},
			UnitRelationData: map[string]params.RelationData{
				"gitlab/2": {
					InScope:  true,
					UnitData: map[string]interface{}{"gitlab/2": "gitlab/2-setting"},
				},
			},
		}},
		ProviderId: "provider-id",
		Address:    "192.168.1.1",
	})
	c.Assert(result.Results[1].Error, jc.DeepEquals, &params.Error{
		Code:    "not found",
		Message: `unit "mysql/0" not found`,
	})
}

func (s *ApplicationSuite) TestUnitsInfoForApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)

	unit0 := s.expectUnitWithCloudContainer(ctrl, s.expectCloudContainer(ctrl), "postgresql/0")
	unit1 := s.expectUnitWithCloudContainer(ctrl, s.expectCloudContainer(ctrl), "postgresql/1")
	app.EXPECT().AllUnits().Return([]application.Unit{unit0, unit1}, nil)

	rel := s.expectRelation(ctrl, "postgresql:db gitlab:server", false)
	rel.EXPECT().Id().Return(101).MinTimes(1)
	rel.EXPECT().AllRemoteUnits("gitlab").Return([]application.RelationUnit{s.expectRelationUnit(ctrl, "gitlab/2")}, nil).MinTimes(1)
	app.EXPECT().Relations().Return([]application.Relation{rel}, nil).MinTimes(1)

	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	s.backend.EXPECT().Machine("0").Return(s.expectMachineWithIP(ctrl, "10.0.0.1"), nil)
	s.backend.EXPECT().Machine("1").Return(s.expectMachineWithIP(ctrl, "10.0.0.1"), nil)

	s.model.EXPECT().OpenedPortRangesForMachine("0").Return(s.expectMachineUnitPortRange(ctrl, "postgresql/0", "100-102/tcp"), nil)
	s.model.EXPECT().OpenedPortRangesForMachine("1").Return(s.expectMachineUnitPortRange(ctrl, "postgresql/1", "100-102/tcp"), nil)

	// gitlab exists is remote in this scenerios so return a not found error
	s.backend.EXPECT().Application("gitlab").Return(nil, errors.NotFoundf(`application "gitlab"`)).MinTimes(1)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.UnitsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.UnitResult{
		Tag:             "unit-postgresql-0",
		WorkloadVersion: "666",
		Machine:         "0",
		OpenedPorts:     []string{"100-102/tcp"},
		PublicAddress:   "10.0.0.1",
		Charm:           "ch:postgresql-42",
		Leader:          true,
		Life:            state.Alive.String(),
		RelationData: []params.EndpointRelationData{{
			RelationId:      101,
			Endpoint:        "db",
			CrossModel:      true,
			RelatedEndpoint: "server",
			ApplicationData: map[string]interface{}{"app-gitlab": "setting"},
			UnitRelationData: map[string]params.RelationData{
				"gitlab/2": {
					InScope:  true,
					UnitData: map[string]interface{}{"gitlab/2": "gitlab/2-setting"},
				},
			},
		}},
		ProviderId: "provider-id",
		Address:    "192.168.1.1",
	})
	c.Assert(*result.Results[1].Result, gc.DeepEquals, params.UnitResult{
		Tag:             "unit-postgresql-1",
		WorkloadVersion: "666",
		Machine:         "1",
		OpenedPorts:     []string{"100-102/tcp"},
		PublicAddress:   "10.0.0.1",
		Charm:           "ch:postgresql-42",
		Leader:          false,
		Life:            state.Alive.String(),
		RelationData: []params.EndpointRelationData{{
			RelationId:      101,
			Endpoint:        "db",
			CrossModel:      true,
			RelatedEndpoint: "server",
			ApplicationData: map[string]interface{}{"app-gitlab": "setting"},
			UnitRelationData: map[string]params.RelationData{
				"gitlab/2": {
					InScope:  true,
					UnitData: map[string]interface{}{"gitlab/2": "gitlab/2-setting"},
				},
			},
		}},
		ProviderId: "provider-id",
		Address:    "192.168.1.1",
	})
}

func (s *ApplicationSuite) TestLeader(c *gc.C) {
	defer s.setup(c).Finish()

	result, err := s.api.Leader(params.Entity{Tag: names.NewApplicationTag("postgresql").String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "postgresql/0")
}

func (s *ApplicationSuite) setupConfigTest(ctrl *gomock.Controller) {
	ch := s.expectCharm(ctrl, nil, nil, &charm.Config{Options: map[string]charm.Option{
		"outlook":     {Type: "string", Description: "No default outlook."},
		"skill-level": {Type: "int", Description: "A number indicating skill."},
		"title":       {Type: "string", Default: "My Title", Description: "A descriptive title used for the application."},
		"username":    {Type: "string", Default: "admin001", Description: "The name of the initial account (given admin permissions)."},
	}})

	foo := s.expectApplicationWithCharm(ctrl, ch, "foo")
	foo.EXPECT().CharmConfig(gomock.Any()).Return(map[string]interface{}{
		"title":       "foo",
		"skill-level": 42,
	}, nil)
	s.backend.EXPECT().Application("foo").Return(foo, nil)

	bar := s.expectApplicationWithCharm(ctrl, ch, "bar")
	bar.EXPECT().CharmConfig(gomock.Any()).Return(map[string]interface{}{
		"title":   "bar",
		"outlook": "fantastic",
	}, nil)
	s.backend.EXPECT().Application("bar").Return(bar, nil)

	s.backend.EXPECT().Application(gomock.Any()).DoAndReturn(func(name string) (application.Application, error) {
		return nil, errors.NotFoundf("application %q", name)
	}).AnyTimes()
}

func (s *ApplicationSuite) TestCharmConfig(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.setupConfigTest(ctrl)

	branch := "test-branch"
	results, err := s.api.CharmConfig(params.ApplicationGetArgs{
		Args: []params.ApplicationGet{
			{ApplicationName: "foo", BranchName: branch},
			{ApplicationName: "bar", BranchName: branch},
			{ApplicationName: "wat", BranchName: branch},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertConfigTest(c, results, []params.ConfigResult{})
}

func (s *ApplicationSuite) TestGetConfig(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.setupConfigTest(ctrl)

	results, err := s.api.GetConfig(params.Entities{
		Entities: []params.Entity{
			{Tag: "wat"}, {Tag: "machine-0"}, {Tag: "user-foo"},
			{Tag: "application-foo"}, {Tag: "application-bar"}, {Tag: "application-wat"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertConfigTest(c, results, []params.ConfigResult{
		{Error: &params.Error{Message: `"wat" is not a valid tag`}},
		{Error: &params.Error{Message: `unexpected tag type, expected application, got machine`}},
		{Error: &params.Error{Message: `unexpected tag type, expected application, got user`}},
	})

}

func assertConfigTest(c *gc.C, results params.ApplicationGetConfigResults, resPrefix []params.ConfigResult) {
	c.Assert(results, jc.DeepEquals, params.ApplicationGetConfigResults{
		Results: append(resPrefix, []params.ConfigResult{
			{
				Config: map[string]interface{}{
					"outlook": map[string]interface{}{
						"description": "No default outlook.",
						"source":      "unset",
						"type":        "string",
					},
					"skill-level": map[string]interface{}{
						"description": "A number indicating skill.",
						"source":      "user",
						"type":        "int",
						"value":       42,
					},
					"title": map[string]interface{}{
						"default":     "My Title",
						"description": "A descriptive title used for the application.",
						"source":      "user",
						"type":        "string",
						"value":       "foo",
					},
					"username": map[string]interface{}{
						"default":     "admin001",
						"description": "The name of the initial account (given admin permissions).",
						"source":      "default",
						"type":        "string",
						"value":       "admin001",
					},
				},
			}, {
				Config: map[string]interface{}{
					"outlook": map[string]interface{}{
						"description": "No default outlook.",
						"source":      "user",
						"type":        "string",
						"value":       "fantastic",
					},
					"skill-level": map[string]interface{}{
						"description": "A number indicating skill.",
						"source":      "unset",
						"type":        "int",
					},
					"title": map[string]interface{}{
						"default":     "My Title",
						"description": "A descriptive title used for the application.",
						"source":      "user",
						"type":        "string",
						"value":       "bar",
					},
					"username": map[string]interface{}{
						"default":     "admin001",
						"description": "The name of the initial account (given admin permissions).",
						"source":      "default",
						"type":        "string",
						"value":       "admin001",
					},
				},
			}, {
				Error: &params.Error{Message: `application "wat" not found`, Code: "not found"},
			},
		}...)})
}

func (s *ApplicationSuite) TestSetMetricCredentialsOneArg(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectApplication(ctrl, "mysql")
	app.EXPECT().SetMetricCredentials([]byte("creds 1234")).Return(nil)
	s.backend.EXPECT().Application("mysql").Return(app, nil)

	results, err := s.api.SetMetricCredentials(params.ApplicationMetricCredentials{Creds: []params.ApplicationMetricCredential{{
		ApplicationName:   "mysql",
		MetricCredentials: []byte("creds 1234"),
	}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *ApplicationSuite) TestSetMetricCredentialsTwoArgsBothPass(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app0 := s.expectApplication(ctrl, "mysql")
	app0.EXPECT().SetMetricCredentials([]byte("creds 1234")).Return(nil)
	s.backend.EXPECT().Application("mysql").Return(app0, nil)

	app1 := s.expectApplication(ctrl, "wordpress")
	app1.EXPECT().SetMetricCredentials([]byte("creds 4567")).Return(nil)
	s.backend.EXPECT().Application("wordpress").Return(app1, nil)

	results, err := s.api.SetMetricCredentials(params.ApplicationMetricCredentials{Creds: []params.ApplicationMetricCredential{{
		ApplicationName:   "mysql",
		MetricCredentials: []byte("creds 1234"),
	}, {
		ApplicationName:   "wordpress",
		MetricCredentials: []byte("creds 4567"),
	}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *ApplicationSuite) TestSetMetricCredentialsTwoArgsSecondFails(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectApplication(ctrl, "mysql")
	app.EXPECT().SetMetricCredentials([]byte("creds 1234")).Return(nil)
	s.backend.EXPECT().Application("mysql").Return(app, nil)
	s.backend.EXPECT().Application("not-an-application").Return(nil, errors.NotFoundf(`application "not-an-application"`))

	results, err := s.api.SetMetricCredentials(params.ApplicationMetricCredentials{Creds: []params.ApplicationMetricCredential{{
		ApplicationName:   "mysql",
		MetricCredentials: []byte("creds 1234"),
	}, {
		ApplicationName:   "not-an-application",
		MetricCredentials: []byte("creds 4567"),
	}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{},
		{Error: &params.Error{Message: `application "not-an-application" not found`, Code: "not found"}},
	}})
}

func (s *ApplicationSuite) TestCompatibleSettingsParsing(c *gc.C) {
	settings, err := application.ParseSettingsCompatible(defaultCharmConfig, map[string]string{
		"stringOption": "",
		"intOption":    "27",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"stringOption": nil,
		"intOption":    int64(27),
	})
}

func (s *ApplicationSuite) TestIllegalSettingsParsing(c *gc.C) {
	_, err := application.ParseSettingsCompatible(defaultCharmConfig, map[string]string{
		"yummy": "didgeridoo",
	})
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

func (s *ApplicationSuite) TestApplicationGetCharmURLOrigin(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	rev := 42
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source:   "local",
		Revision: &rev,
		Channel: &state.Channel{
			Track:  "latest",
			Risk:   "stable",
			Branch: "foo",
		},
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	result, err := s.api.GetCharmURLOrigin(params.ApplicationGet{ApplicationName: "postgresql"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.URL, gc.Equals, "ch:postgresql-42")

	latest := "latest"
	branch := "foo"

	c.Assert(result.Origin, jc.DeepEquals, params.CharmOrigin{
		Source:       "local",
		Risk:         "stable",
		Revision:     &rev,
		Track:        &latest,
		Branch:       &branch,
		Architecture: "amd64",
		Base:         params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		InstanceKey:  charmhub.CreateInstanceKey(app.ApplicationTag(), coretesting.ModelTag),
	})
}
