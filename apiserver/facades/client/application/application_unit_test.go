// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"regexp"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/assumes"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
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

	api *application.APIv15

	backend            *mocks.MockBackend
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

	deployParams               map[string]application.DeployApplicationParams
	addRemoteApplicationParams state.AddRemoteApplicationParams
	consumeApplicationArgs     params.ConsumeApplicationArgs
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

	testMac := apitesting.MustNewMacaroon("test")
	s.addRemoteApplicationParams = state.AddRemoteApplicationParams{
		Name:        "hosted-mysql",
		OfferUUID:   "hosted-mysql-uuid",
		URL:         "othermodel.hosted-mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints:   []charm.Relation{{Name: "database", Interface: "mysql", Role: "provider"}},
		Spaces:      []*environs.ProviderSpaceInfo{},
		Macaroon:    testMac,
	}

	s.consumeApplicationArgs = params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
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
		s.blockChecker,
		s.model,
		s.leadershipReader,
		func(application.Charm) *state.Charm {
			//return &state.Charm{}
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
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = &application.APIv15{api}
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

func (s *ApplicationSuite) expectDefaultCharm(ctrl *gomock.Controller) *mocks.MockCharm {
	return s.expectCharm(ctrl, &charm.Meta{Name: "charm-postgresql"}, &charm.Manifest{}, &charm.Config{Options: map[string]charm.Option{
		"stringOption": {Type: "string"},
		"intOption":    {Type: "int", Default: int(123)},
	}})
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
	var series string
	if s.modelType == state.ModelTypeIAAS {
		series = "quantal"
	}
	if s.modelType == state.ModelTypeCAAS {
		series = "kubernetes"
	}
	app := mocks.NewMockApplication(ctrl)
	app.EXPECT().Name().Return(name).AnyTimes()
	app.EXPECT().Series().Return(series).AnyTimes()
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

func (s *ApplicationSuite) TestUpdateCAASApplicationSettings(c *gc.C) {
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
	args := params.ApplicationUpdate{
		ApplicationName: "postgresql",
		SettingsYAML:    "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}
	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	err := api.Update(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestUpdateCAASApplicationSettingsInIAASModelTriggersError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "postgresql",
		SettingsYAML:    "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}
	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	err := api.Update(args)
	c.Assert(err, gc.ErrorMatches, `.*unknown option "juju-external-hostname"`, gc.Commentf("expected to get an error when attempting to set CAAS-specific app setting in IAAS model"))
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

func (s *ApplicationSuite) TestSetCharmStorageConstraints(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		StorageConstraints: map[string]state.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: 123},
			"d": {Count: 456},
		},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		StorageConstraints: map[string]params.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: toUint64Ptr(123)},
			"d": {Count: toUint64Ptr(456)},
		},
		CharmOrigin: &params.CharmOrigin{Source: "charm-store", Series: "bionic"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCAASCharmInvalid(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{Deployment: &charm.Deployment{}}, nil, nil)
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		CharmOrigin:     &params.CharmOrigin{OS: "ubuntu", Series: "bionic"},
	})
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "Juju on containers does not support updating deployment info.*")
}

func (s *ApplicationSuite) TestApplicationSetCharmV12(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Type:     "charm",
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "quantal"},
		},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)
	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	err := api.SetCharm(params.ApplicationSetCharmV12{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
	})
	c.Assert(err, jc.ErrorIsNil)
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
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     &params.CharmOrigin{Source: "charm-store", OS: "ubuntu", Series: "bionic"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmDisallowDowngradeFormat(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := charm.MustParseURL("ch:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectCharm(ctrl, &charm.Meta{Name: "charm-postgresql"}, &charm.Manifest{Bases: []charm.Base{{}}}, nil)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		CharmOrigin:     &params.CharmOrigin{Series: "quantal"},
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
			Track: "20.04",
			Risk:  "stable",
		},
	}}}, nil)
	curl := charm.MustParseURL("ch:postgresql")
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
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		Series:         "focal",
		RequireNoUnits: true,
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"operator-storage": "k8s-operator-storage",
		"workload-storage": "k8s-storage",
		"default-series":   "focal",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs))

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Series: "bionic"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmConfigSettingsYAML(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-store", Series: "bionic"},
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
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-store", Series: "bionic"},
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithOldAgentVersion(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultLxdProfilerCharm(ctrl)
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 5, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		CharmOrigin:     &params.CharmOrigin{Series: "bionic"},
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, gc.ErrorMatches, "Unable to upgrade LXDProfile charms with the current model version. "+
		"Please run juju upgrade-juju to upgrade the current model to match your controller.")
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithEmptyProfile(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectLxdProfilerCharm(ctrl, &charm.LXDProfile{})
	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	currentCh := s.expectDefaultLxdProfilerCharm(ctrl)
	app := s.expectApplicationWithCharm(ctrl, currentCh, "postgresql")
	app.EXPECT().AgentTools().Return(&agentTools, nil)
	cfg := state.SetCharmConfig{
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{OS: "ubuntu", Series: "bionic"},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.model.EXPECT().AgentVersion().Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     &params.CharmOrigin{Source: "charm-store", Series: "bionic"},
	})
	c.Assert(err, jc.ErrorIsNil)
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

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)
	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return(nil, nil)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{}).Return(nil)

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{ApplicationTag: "application-postgresql"}},
	})
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

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)
	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return(nil, nil)

	zero := time.Duration(0)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{ForcedOperation: state.ForcedOperation{
		Force:   true,
		MaxWait: common.MaxWait(&zero),
	}}).Return(nil)

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			Force:          true,
			MaxWait:        &zero,
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
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	})
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

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)
	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return(nil, nil)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{
		DestroyStorage: true,
	}).Return(nil)

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

func (s *ApplicationSuite) TestDestroyApplicationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().Application("postgresql").Return(nil, errors.NotFoundf(`application "postgresql"`))

	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{
			{ApplicationTag: "application-postgresql"},
		},
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

	// unit 0 loop
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)

	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{}).Return(nil)

	// unit 1 loop
	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	unit1.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return([]state.StorageAttachment{}, nil)

	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

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

func (s *ApplicationSuite) TestForceDestroyUnit(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	// unit 0 loop
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation().Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/0")).Return([]state.StorageAttachment{
		s.expectStorageAttachment(ctrl, "pgdata/0"),
		s.expectStorageAttachment(ctrl, "pgdata/1"),
	}, nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/0")).Return(s.expectStorageInstance(ctrl, "pgdata/0"), nil)
	s.storageAccess.EXPECT().StorageInstance(names.NewStorageTag("pgdata/1")).Return(s.expectStorageInstance(ctrl, "pgdata/1"), nil)

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

	s.storageAccess.EXPECT().UnitStorageAttachments(names.NewUnitTag("postgresql/1")).Return([]state.StorageAttachment{}, nil)

	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

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

func (s *ApplicationSuite) TestDeployAttachStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(3)
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
			AttachStorage:   []string{"storage-foo-0"},
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-1",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        2,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-2",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(3)
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	track := "latest"
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
		}, {
			ApplicationName: "bar",
			CharmURL:        "cs:bar-0",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-store",
				Risk:   "stable",
				Track:  &track,
				Series: "bionic",
			},
			NumUnits: 1,
		}, {
			ApplicationName: "hub",
			CharmURL:        "hub-0",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-hub",
				Risk:   "stable",
				Series: "bionic",
			},
			NumUnits: 1,
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.IsNil)

	c.Assert(s.deployParams["foo"].CharmOrigin.Source, gc.Equals, corecharm.Source("local"))
	c.Assert(s.deployParams["bar"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-store"))
	c.Assert(s.deployParams["hub"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-hub"))
}

// Some clients we need to support deploy applications without any OS data in the
// charm origin. (juju 2.8 does not understand the concept of a charm origin; pylibjuju
// 3.0 does, but misses the series/base attributes). Instead it is provided via the
// Series key.
func (s *ApplicationSuite) TestDeploySeriesInArgOnly(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(3)
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	track := "latest"
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			Series:          "bionic",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}, {
			ApplicationName: "bar",
			CharmURL:        "cs:bar-0",
			Series:          "bionic",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-store",
				Risk:   "stable",
				Track:  &track,
			},
			NumUnits: 1,
		}, {
			ApplicationName: "hub",
			CharmURL:        "hub-0",
			Series:          "bionic",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-hub",
				Risk:   "stable",
			},
			NumUnits: 1,
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.IsNil)

	c.Assert(s.deployParams["foo"].CharmOrigin.Source, gc.Equals, corecharm.Source("local"))
	c.Assert(s.deployParams["bar"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-store"))
	c.Assert(s.deployParams["hub"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-hub"))
}

func (s *ApplicationSuite) TestDeployInconsistentSeries(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			Series:          "bionic",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "focal"},
			NumUnits:        1,
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `.*inconsistent values for series detected.*`)
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

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)
	s.expectDefaultK8sModelConfig()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-annotations": "a=b c="},
			ConfigYAML:      "foo:\n  stringOption: fred\n  kubernetes-service-type: loadbalancer",
		}, {
			ApplicationName: "foobar",
			CharmURL:        "local:foobar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-type": "cluster", "intOption": "2"},
			ConfigYAML:      "foobar:\n  intOption: 1\n  kubernetes-service-type: loadbalancer\n  kubernetes-ingress-ssl-redirect: true",
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
			NumUnits:        1,
			Placement:       []*instance.Placement{{}, {}},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 4)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.ErrorMatches, "AttachStorage may not be specified for container models")
	c.Assert(results.Results[3].Error, gc.ErrorMatches, "only 1 placement directive is supported for container models, got 2")

	c.Assert(s.deployParams["foo"].ApplicationConfig.Attributes()["kubernetes-service-type"], gc.Equals, "loadbalancer")
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

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Series: "bionic"},
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
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	attrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).MinTimes(1)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)
	s.expectDefaultK8sModelConfig()
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any()).Return(nil)
	s.storagePoolManager.EXPECT().Get("k8s-operator-storage").Return(nil, errors.NotFoundf("pool"))
	s.registry.EXPECT().StorageProvider(storage.ProviderType("k8s-operator-storage")).Return(nil, errors.NotFoundf(`provider type "k8s-operator-storage"`))

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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
	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Series: "bionic"},
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

var unifiedSeriesTests = []struct {
	desc     string
	param    params.ApplicationDeploy
	unified  string
	errMatch string
}{
	{
		desc:    "Series only",
		param:   params.ApplicationDeploy{Series: "focal", CharmURL: "ch:foo"},
		unified: "focal",
	},
	{
		desc: "All present",
		param: params.ApplicationDeploy{
			Series:      "focal",
			CharmURL:    "ch:focal/foo",
			CharmOrigin: &params.CharmOrigin{Series: "focal", Base: params.Base{Name: "ubuntu", Channel: "20.04"}},
		},
		unified: "focal",
	},
	{
		desc: "Clash",
		param: params.ApplicationDeploy{
			Series:      "jammy",
			CharmURL:    "ch:foo",
			CharmOrigin: &params.CharmOrigin{Series: "focal"},
		},
		errMatch: `.*inconsistent values for series detected. argument: "jammy", charm origin series: "focal".*`,
	},
	{
		desc: "Clash with base",
		param: params.ApplicationDeploy{
			Series:      "jammy",
			CharmURL:    "ch:foo",
			CharmOrigin: &params.CharmOrigin{Base: params.Base{Name: "ubuntu", Channel: "20.04"}},
		},
		errMatch: `.*inconsistent values for series detected. argument: "jammy".* charm origin base: "ubuntu@20.04".*`,
	},
	{
		desc: "No series",
		param: params.ApplicationDeploy{
			ApplicationName: "foo",
			CharmURL:        "ch:foo",
			CharmOrigin:     &params.CharmOrigin{},
		},
		errMatch: `unable to determine series for "foo"`,
	},
}

func (s *ApplicationSuite) TestGetUnifiedSeries(c *gc.C) {
	for i, t := range unifiedSeriesTests {
		c.Logf("Test %d: %s", i, t.desc)
		unified, err := application.GetUnifiedSeries(t.param)
		if t.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(unified, gc.Equals, t.unified)
		} else {
			c.Check(err, gc.ErrorMatches, t.errMatch)
		}
	}
}

func (s *ApplicationSuite) TestAddUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	newUnit := s.expectUnit(ctrl, "postgresql/99")
	newUnit.EXPECT().AssignWithPolicy(state.AssignCleanEmpty)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AddUnit(state.AddUnitParams{AttachStorage: []names.StorageTag{}}).Return(newUnit, nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

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
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
		AttachStorage:   []string{"storage-pgdata-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
		AttachStorage:   []string{"storage-foo-0"},
	})
	c.Assert(err, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageInvalidStorageTag(c *gc.C) {
	defer s.setup(c).Finish()

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
	s.addRemoteApplicationParams.Bindings = map[string]string{"server": "myspace"}
	s.addRemoteApplicationParams.Spaces = []*environs.ProviderSpaceInfo{{
		CloudType:          "grandaddy",
		ProviderAttributes: map[string]interface{}{"thunderjaws": 1},
		SpaceInfo: network.SpaceInfo{
			Name:       network.SpaceName("myspace"),
			ProviderId: network.Id("juju-space-myspace"),
			Subnets: []network.SubnetInfo{{
				CIDR:              "5.6.7.0/24",
				ProviderId:        network.Id("juju-subnet-1"),
				AvailabilityZones: []string{"az1"},
			}},
		},
	}}

	s.consumeApplicationArgs.Args[0].ApplicationAlias = "beirut"
	s.consumeApplicationArgs.Args[0].ApplicationOfferDetails.Bindings = map[string]string{"server": "myspace"}
	s.consumeApplicationArgs.Args[0].ApplicationOfferDetails.Spaces = []params.RemoteSpace{{
		CloudType:  "grandaddy",
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		ProviderAttributes: map[string]interface{}{
			"thunderjaws": 1,
		},
		Subnets: []params.Subnet{{
			CIDR:       "5.6.7.0/24",
			ProviderId: "juju-subnet-1",
			Zones:      []string{"az1"},
		}},
	}}

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

	s.consumeApplicationArgs.Args[0].ApplicationOfferDetails.SourceModelTag = names.NewModelTag(utils.MustNewUUID().String()).String()

	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
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

func (s *ApplicationSuite) TestApplicationUpdateSeriesNoParams(c *gc.C) {
	defer s.setup(c).Finish()

	api := application.APIv14{s.api}
	results, err := api.UpdateApplicationSeries(
		params.UpdateChannelArgs{
			Args: []params.UpdateChannelArg{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{}})
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
				Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
				Series: "trusty",
			}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestRemoteRelationBadCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"bad.cidr"}})
	c.Assert(err, gc.ErrorMatches, `invalid CIDR address: bad.cidr`)
}

func (s *ApplicationSuite) TestRemoteRelationDisAllowedCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"0.0.0.0/0"}})
	c.Assert(err, gc.ErrorMatches, `CIDR "0.0.0.0/0" not allowed`)
}

func (s *ApplicationSuite) TestSetApplicationConfigExplicitMaster(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig(model.GenerationMaster, charm.Settings{"stringOption": "stringVal"})
	s.expectUpdateApplicationConfig(c, app)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "foo",
				"stringOption":           "stringVal",
			},
			Generation: model.GenerationMaster,
		}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetApplicationConfigEmptyUsesMaster(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig(model.GenerationMaster, charm.Settings{"stringOption": "stringVal"})
	s.expectUpdateApplicationConfig(c, app)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "foo",
				"stringOption":           "stringVal",
			},
		}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetApplicationConfigBranch(c *gc.C) {
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

	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
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

func (s *ApplicationSuite) TestSetApplicationsEmptyConfigMasterBranch(c *gc.C) {
	s.modelType = state.ModelTypeCAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateCharmConfig("master", charm.Settings{"stringOption": ""})
	s.expectUpdateApplicationConfig(c, app)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
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

func (s *ApplicationSuite) TestSetApplicationConfigPermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	s.modelType = state.ModelTypeCAAS
	defer s.setup(c).Finish()

	api := &application.APIv12{&application.APIv13{&application.APIv14{s.api}}}
	_, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
		}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
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

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Channel: &state.Channel{Track: "2.0", Risk: "candidate"},
	}).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)
	app.EXPECT().Channel().Return(csparams.DevelopmentChannel).MinTimes(1)

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
		Series:      "quantal",
		Base:        params.Base{Name: "ubuntu", Channel: "12.10/stable"},
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{{ID: "42", Name: "non-euclidean-geometry"}}, nil).MinTimes(1)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(nil).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)
	app.EXPECT().Channel().Return(csparams.DevelopmentChannel).MinTimes(1)

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
		Series:      "quantal",
		Base:        params.Base{Name: "ubuntu", Channel: "12.10/stable"},
		Channel:     "development",
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

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

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

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

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

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil).MinTimes(1)

	// postgresql
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().CharmOrigin().Return(nil).MinTimes(1)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig("master").Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)
	app.EXPECT().Channel().Return(csparams.DevelopmentChannel).MinTimes(1)

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
		Series:      "quantal",
		Base:        params.Base{Name: "ubuntu", Channel: "12.10/stable"},
		Channel:     "development",
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

	s.backend.EXPECT().AllSpaceInfos().Return(network.SpaceInfos{}, nil)

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

func (s *ApplicationSuite) expectMachine(ctrl *gomock.Controller, publicAddress string) *mocks.MockMachine {
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().PublicAddress().Return(network.SpaceAddress{MachineAddress: network.MachineAddress{Value: publicAddress}}, nil).AnyTimes()
	return machine
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

func (s *ApplicationSuite) TestUnitsInfo(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unit := s.expectUnitWithCloudContainer(ctrl, s.expectCloudContainer(ctrl), "postgresql/0")
	s.backend.EXPECT().Unit("postgresql/0").Return(unit, nil)

	s.backend.EXPECT().Unit("mysql/0").Return(nil, errors.NotFoundf(`unit "mysql/0"`))

	app := s.expectDefaultApplication(ctrl)
	curl := "cs:postgresql-42"
	app.EXPECT().CharmURL().Return(&curl, true)

	rel := s.expectRelation(ctrl, "postgresql:db gitlab:server", false)
	rel.EXPECT().Id().Return(101)
	rel.EXPECT().AllRemoteUnits("gitlab").Return([]application.RelationUnit{s.expectRelationUnit(ctrl, "gitlab/2")}, nil)
	app.EXPECT().Relations().Return([]application.Relation{rel}, nil)

	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.backend.EXPECT().Machine("0").Return(s.expectMachine(ctrl, "10.0.0.1"), nil)

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
		Charm:           "cs:postgresql-42",
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

	curl := "cs:postgresql-42"
	app.EXPECT().CharmURL().Return(&curl, true).MinTimes(1)

	rel := s.expectRelation(ctrl, "postgresql:db gitlab:server", false)
	rel.EXPECT().Id().Return(101).MinTimes(1)
	rel.EXPECT().AllRemoteUnits("gitlab").Return([]application.RelationUnit{s.expectRelationUnit(ctrl, "gitlab/2")}, nil).MinTimes(1)
	app.EXPECT().Relations().Return([]application.Relation{rel}, nil).MinTimes(1)

	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	s.backend.EXPECT().Machine("0").Return(s.expectMachine(ctrl, "10.0.0.1"), nil)
	s.backend.EXPECT().Machine("1").Return(s.expectMachine(ctrl, "10.0.0.1"), nil)

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
		Charm:           "cs:postgresql-42",
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
		Charm:           "cs:postgresql-42",
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

	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Try to upgrade the charm
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		CharmOrigin:     &params.CharmOrigin{Series: "bionic"},
	})
	c.Assert(err, gc.ErrorMatches, `(?m).*Charm feature requirements cannot be met.*`)
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

	curl := charm.MustParseURL("cs:postgresql")
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetCharm(gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Try to upgrade the charm
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
		Force:           true,
		CharmOrigin:     &params.CharmOrigin{Series: "bionic"},
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("expected SetCharm to succeed when --force is set"))
}

func (s *ApplicationSuite) TestLeader(c *gc.C) {
	defer s.setup(c).Finish()

	result, err := s.api.Leader(params.Entity{Tag: names.NewApplicationTag("postgresql").String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "postgresql/0")
}

func (s *ApplicationSuite) TestApplicationGetCharmURL(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	curl := "local:quantal/postgresql-3"
	app.EXPECT().CharmURL().Return(&curl, false)
	result, err := s.api.GetCharmURL(params.ApplicationGet{ApplicationName: "postgresql"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "local:quantal/postgresql-3")
}

func (s *ApplicationSuite) TestApplicationGetCharmURLOrigin(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	// Technically this charm origin is impossible, a local
	// charm cannot have a channel.  Done just for testing.
	rev := 666
	stateOrigin := state.CharmOrigin{
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
			Series:       "focal",
		},
	}
	curl := "local:quantal/postgresql-666"
	app.EXPECT().ApplicationTag().Return(names.NewApplicationTag("postgresql")).AnyTimes()
	app.EXPECT().CharmURL().Return(&curl, false)
	app.EXPECT().CharmOrigin().Return(&stateOrigin)
	result, err := s.api.GetCharmURLOrigin(params.ApplicationGet{ApplicationName: "postgresql"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.URL, gc.Equals, curl)

	latest := "latest"
	branch := "foo"

	c.Assert(result.Origin, jc.DeepEquals, params.CharmOrigin{
		Source:       "local",
		Risk:         "stable",
		Revision:     &rev,
		Track:        &latest,
		Branch:       &branch,
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
		Channel:      "20.04/stable",
		Base:         params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		InstanceKey:  charmhub.CreateInstanceKey(app.ApplicationTag(), s.model.ModelTag()),
	})
}

// TODO(juju3) - delete me
func (s *ApplicationSuite) TestApplicationGetCharmURLOriginMissingOS(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	stateOrigin := state.CharmOrigin{
		Source: "local",
		Platform: &state.Platform{
			Architecture: "amd64",
			Series:       "focal",
		},
	}
	curl := "local:quantal/postgresql-666"
	app.EXPECT().ApplicationTag().Return(names.NewApplicationTag("postgresql")).AnyTimes()
	app.EXPECT().CharmURL().Return(&curl, false)
	app.EXPECT().CharmOrigin().Return(&stateOrigin)
	result, err := s.api.GetCharmURLOrigin(params.ApplicationGet{ApplicationName: "postgresql"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.URL, gc.Equals, curl)

	c.Assert(result.Origin, jc.DeepEquals, params.CharmOrigin{
		Source:       "local",
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
		Channel:      "20.04/stable",
		Base:         params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		InstanceKey:  charmhub.CreateInstanceKey(app.ApplicationTag(), s.model.ModelTag()),
	})
}
