// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/application/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	coreassumes "github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charmhub"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type mockLXDProfilerCharm struct {
	*mocks.MockCharm
	*mocks.MockLXDProfiler
}

type ApplicationSuite struct {
	testing.CleanupSuite

	api *application.APIBase

	backend            *mocks.MockBackend
	ecService          *application.MockExternalControllerService
	modelConfigService *application.MockModelConfigService
	modelAgentService  *application.MockModelAgentService
	cloudService       *commonmocks.MockCloudService
	credService        *commonmocks.MockCredentialService
	machineService     *application.MockMachineService
	applicationService *application.MockApplicationService
	storageAccess      *mocks.MockStorageInterface
	model              *mocks.MockModel
	leadershipReader   *mocks.MockReader
	storagePoolGetter  *application.MockStoragePoolGetter
	registry           *mocks.MockProviderRegistry
	caasBroker         *mocks.MockCaasBrokerInterface
	store              *mocks.MockObjectStore
	networkService     *application.MockNetworkService

	blockChecker  *mocks.MockBlockChecker
	changeAllowed error
	removeAllowed error

	authorizer  apiservertesting.FakeAuthorizer
	modelInfo   model.ReadOnlyModel
	modelConfig coretesting.Attrs

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

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.modelInfo = model.ReadOnlyModel{
		UUID: model.UUID(coretesting.ModelTag.Id()),
		Type: model.IAAS,
	}
	s.modelConfig = coretesting.FakeConfig()
	s.PatchValue(&application.ClassifyDetachedStorage, fakeClassifyDetachedStorage)
	s.deployParams = make(map[string]application.DeployApplicationParams)

	s.changeAllowed = nil
	s.removeAllowed = nil

	s.allSpaceInfos = network.SpaceInfos{}

	testMac := jujutesting.MustNewMacaroon("test")
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

	s.networkService = application.NewMockNetworkService(ctrl)
	s.backend = mocks.NewMockBackend(ctrl)
	s.backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.machineService = application.NewMockMachineService(ctrl)
	s.applicationService = application.NewMockApplicationService(ctrl)

	var fs coreassumes.FeatureSet
	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()

	s.ecService = application.NewMockExternalControllerService(ctrl)

	s.storageAccess = mocks.NewMockStorageInterface(ctrl)
	s.storageAccess.EXPECT().VolumeAccess().Return(nil).AnyTimes()
	s.storageAccess.EXPECT().FilesystemAccess().Return(nil).AnyTimes()

	s.blockChecker = mocks.NewMockBlockChecker(ctrl)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(s.changeAllowed).AnyTimes()
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(s.removeAllowed).AnyTimes()

	s.model = mocks.NewMockModel(ctrl)
	s.backend.EXPECT().Model().Return(s.model, nil).AnyTimes()

	s.leadershipReader = mocks.NewMockReader(ctrl)
	s.leadershipReader.EXPECT().Leaders().Return(map[string]string{
		"postgresql": "postgresql/0",
	}, nil).AnyTimes()

	s.storagePoolGetter = application.NewMockStoragePoolGetter(ctrl)

	s.registry = mocks.NewMockProviderRegistry(ctrl)
	s.caasBroker = mocks.NewMockCaasBrokerInterface(ctrl)
	ver := version.MustParse("1.15.0")
	s.caasBroker.EXPECT().Version().Return(&ver, nil).AnyTimes()

	s.modelConfigService = application.NewMockModelConfigService(ctrl)
	cfg, err := config.New(config.UseDefaults, s.modelConfig)
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()

	s.modelAgentService = application.NewMockModelAgentService(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.credService = commonmocks.NewMockCredentialService(ctrl)

	s.store = mocks.NewMockObjectStore(ctrl)

	api, err := application.NewAPIBase(
		s.backend,
		s.ecService,
		s.networkService,
		s.storageAccess,
		s.authorizer,
		nil,
		s.blockChecker,
		s.model,
		s.modelInfo,
		s.modelConfigService,
		s.modelAgentService,
		s.cloudService,
		s.credService,
		s.machineService,
		s.applicationService,
		s.leadershipReader,
		func(application.Charm) *state.Charm {
			return nil
		},
		func(_ context.Context, _ application.ApplicationDeployer, _ application.Model, _ model.ReadOnlyModel, _ application.ApplicationService, _ objectstore.ObjectStore, p application.DeployApplicationParams, _ corelogger.Logger) (application.Application, error) {
			s.deployParams[p.ApplicationName] = p
			return nil, nil
		},
		s.storagePoolGetter,
		s.registry,
		common.NewResources(),
		s.caasBroker,
		s.store,
		loggertesting.WrapCheckLog(c),
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
	app.EXPECT().Constraints().Return(constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"), nil).AnyTimes()
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), name).Return(life.Alive, nil).AnyTimes()
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

func (s *ApplicationSuite) TestSetCharm(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:something-else"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	cfg := state.SetCharmConfig{CharmOrigin: createStateCharmOriginFromURL(curl)}
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}, s.store)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}, s.store)

	schemaFields, defaults, err := application.ConfigSchema()
	c.Assert(err, jc.ErrorIsNil)
	app.EXPECT().UpdateApplicationConfig(coreconfig.ConfigAttributes{"trust": true}, nil, schemaFields, defaults)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err = s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "postgresql",
		CharmURL:        "ch:something-else",
		CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
	})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestSetCharmRejectCharmStore(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}, s.store)

	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "badapplication",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		ForceBase:       true,
		ForceUnits:      true,
	})
	c.Assert(err, gc.ErrorMatches, `application "badapplication" not found`)
}

func (s *ApplicationSuite) TestSetCharmStorageDirectives(c *gc.C) {
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
	app.EXPECT().SetCharm(setCharmConfigMatcher{c: c, expected: cfg}, s.store)

	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm: ch,
		Storage: map[string]storage.Directive{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: 123},
			"d": {Count: 456},
		},
	}).Return(nil)

	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}
	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "postgresql",
		CharmURL:        "ch:postgresql",
		StorageDirectives: map[string]params.StorageDirectives{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: toUint64Ptr(123)},
			"d": {Count: toUint64Ptr(456)},
		},
		CharmOrigin: createCharmOriginFromURL(curl),
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
	curl := "ch:postgresql"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().SetCharm(state.SetCharmConfig{
		CharmOrigin:    createStateCharmOriginFromURL(curl),
		ConfigSettings: charm.Settings{"stringOption": "value"},
	}, gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "postgresql",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
	})
	c.Assert(err, gc.ErrorMatches, "cannot downgrade from v2 charm format to v1")
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
	}, gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	}, gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.modelAgentService.EXPECT().GetModelAgentVersion(gomock.Any()).Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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

	s.modelAgentService.EXPECT().GetModelAgentVersion(gomock.Any()).Return(version.Number{Major: 2, Minor: 5, Patch: 0}, nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	}, gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.modelAgentService.EXPECT().GetModelAgentVersion(gomock.Any()).Return(version.Number{Major: 2, Minor: 6, Patch: 0}, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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
	app.EXPECT().SetCharm(gomock.Any(), gomock.Any()).Return(nil)
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), "postgresql", applicationservice.UpdateCharmParams{
		Charm:   ch,
		Storage: nil,
	}).Return(nil)

	// Try to upgrade the charm
	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
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

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyRelationNoRelationsFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferActiveRelation("a", "b").Return(nil, errors.New("no relations found"))

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *ApplicationSuite) TestDestroyRelationRelationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferActiveRelation("a:b", "c:d").Return(nil, errors.NotFoundf(`relation "a:b c:d"`))

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{Endpoints: []string{"a:b", "c:d"}})
	c.Assert(err, gc.ErrorMatches, `relation "a:b c:d" not found`)
}

func (s *ApplicationSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	s.removeAllowed = errors.New("remove blocked")
	defer s.setup(c).Finish()

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "remove blocked")
}

func (s *ApplicationSuite) TestDestroyRelationId(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	relation := mocks.NewMockRelation(ctrl)
	relation.EXPECT().DestroyWithForce(false, gomock.Any())
	s.backend.EXPECT().Relation(123).Return(relation, nil)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{RelationId: 123})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyRelationIdRelationNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().Relation(123).Return(nil, errors.NotFoundf(`relation "123"`))

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{RelationId: 123})
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
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), name).Return(life.Alive, nil).AnyTimes()
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

	s.applicationService.EXPECT().DestroyApplication(gomock.Any(), "postgresql")
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{}).Return(nil)

	results, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
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

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
		}},
	})
	c.Assert(err, gc.ErrorMatches, "remove blocked")
}

func (s *ApplicationSuite) TestForceDestroyApplication(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().DestroyApplication(gomock.Any(), "postgresql")
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	zero := time.Duration(0)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{ForcedOperation: state.ForcedOperation{
		Force:   true,
		MaxWait: common.MaxWait(&zero),
	}}).Return(nil)

	results, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
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

	s.applicationService.EXPECT().DestroyApplication(gomock.Any(), "postgresql")
	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AllUnits().Return([]application.Unit{
		s.expectUnit(ctrl, "postgresql/0"),
		s.expectUnit(ctrl, "postgresql/1"),
	}, nil)
	app.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyApplicationOperation{})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	s.expectDefaultStorageAttachments(ctrl)

	s.backend.EXPECT().ApplyOperation(&state.DestroyApplicationOperation{
		DestroyStorage: true,
	}).Return(nil)

	results, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
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
	results, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
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

	results, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{
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

	results, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{
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

	results, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{
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

	results, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{
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
	s.applicationService.EXPECT().DestroyUnit(gomock.Any(), "postgresql/0")
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{}).Return(nil)

	// unit 1 loop
	s.applicationService.EXPECT().DestroyUnit(gomock.Any(), "postgresql/1")
	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	unit1.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

	s.expectDefaultStorageAttachments(ctrl)

	results, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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
	s.applicationService.EXPECT().DestroyUnit(gomock.Any(), "postgresql/0")
	unit0 := s.expectUnit(ctrl, "postgresql/0")
	unit0.EXPECT().IsPrincipal().Return(true)
	unit0.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/0").Return(unit0, nil)

	zero := time.Duration(0)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{ForcedOperation: state.ForcedOperation{
		Force:   true,
		MaxWait: common.MaxWait(&zero),
	}}).Return(nil)

	// unit 1 loop
	s.applicationService.EXPECT().DestroyUnit(gomock.Any(), "postgresql/1")
	unit1 := s.expectUnit(ctrl, "postgresql/1")
	unit1.EXPECT().IsPrincipal().Return(true)
	unit1.EXPECT().DestroyOperation(gomock.Any()).Return(&state.DestroyUnitOperation{})
	s.backend.EXPECT().Unit("postgresql/1").Return(unit1, nil)
	s.backend.EXPECT().ApplyOperation(&state.DestroyUnitOperation{DestroyStorage: true}).Return(nil)

	s.expectDefaultStorageAttachments(ctrl)

	results, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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

	results, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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

	results, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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
	results, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, ".*AttachStorage is non-empty, but NumUnits is 2")
	c.Assert(results.Results[2].Error, gc.ErrorMatches, `.*"volume-baz-0" is not a valid volume tag`)
}

func (s *ApplicationSuite) TestDeployCharmOrigin(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(2)

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
	results, err := s.api.Deploy(context.Background(), args)
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
		return &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}, Architecture: "amd64"}
	default:
		return &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}, Architecture: "amd64"}
	}
}

func createStateCharmOriginFromURL(url string) *state.CharmOrigin {
	curl := charm.MustParseURL(url)
	switch curl.Schema {
	case "local":
		return &state.CharmOrigin{Source: "local", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"}}
	default:
		return &state.CharmOrigin{Source: "charm-hub", Platform: &state.Platform{OS: "ubuntu", Channel: "22.04", Architecture: "amd64"}}
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

	storageDirectives := map[string]storage.Directive{
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
		Storage:         storageDirectives,
	}
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Storage, gc.DeepEquals, storageDirectives)
}

func (s *ApplicationSuite) TestApplicationDeployDefaultFilesystemStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{Storage: map[string]charm.Storage{
		"data": {Type: charm.StorageFilesystem, ReadOnly: true},
	}}, nil, &charm.Config{})
	curl := "ch:utopic/storage-filesystem-1"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
	}
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
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
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Placement, gc.DeepEquals, placement)
}

func validCharmOriginForTest(revision *int) *params.CharmOrigin {
	return &params.CharmOrigin{Source: "charm-hub", Revision: revision}
}

func (s *ApplicationSuite) TestApplicationDeployPlacementModelUUIDSubstitute(c *gc.C) {
	s.modelInfo.UUID = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	curl := "ch:precise/dummy-42"
	s.backend.EXPECT().Charm(curl).Return(ch, nil)

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
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	c.Assert(s.deployParams["my-app"].Placement, gc.DeepEquals, []*instance.Placement{
		{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "0"},
	})
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
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
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
	s.backend.EXPECT().Resources(gomock.Any()).Return(resourcesManager)

	rev := 8
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			// CharmURL is missing & revision included to ensure deployApplication
			// fails sufficiently far through execution so that we can assert pending
			// app resources are removed
			ApplicationName: "my-app",
			CharmOrigin:     validCharmOriginForTest(&rev),
			Resources:       resources,
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

	withTrust := map[string]string{"trust": "true"}
	args := params.ApplicationDeploy{
		ApplicationName: "my-app",
		CharmURL:        curl,
		CharmOrigin:     createCharmOriginFromURL(curl),
		NumUnits:        1,
		Config:          withTrust,
	}
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
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
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "a-space").
		AnyTimes().
		Return(&network.SpaceInfo{
			ID:   "a-space-ID",
			Name: "a-space",
		}, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "").
		AnyTimes().
		Return(nil, errors.New("boom"))
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "42").
		Return(&network.SpaceInfo{
			ID:   "42-ID",
			Name: "42",
		}, nil)

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
	results, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: args,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, ".*boom")
	c.Assert(results.Results[1].Error, gc.IsNil)

	c.Assert(s.deployParams["regular"].EndpointBindings, gc.DeepEquals, map[string]string{
		"endpoint": "42-ID",
	})
}

func (s *ApplicationSuite) addDefaultK8sModelConfig() {
	s.modelConfig = s.modelConfig.Merge(map[string]interface{}{
		"operator-storage": "k8s-operator-storage",
		"workload-storage": "k8s-storage",
	})
}

func (s *ApplicationSuite) TestDeployCAASModel(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.addDefaultK8sModelConfig()
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

	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil).Times(3)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
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
	results, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, ".*AttachStorage may not be specified for container models")
	c.Assert(results.Results[2].Error, gc.ErrorMatches, ".*only 1 placement directive is supported for container models, got 2")
}

func (s *ApplicationSuite) TestDeployCAASBlockStorageRejected(c *gc.C) {
	s.modelInfo.Type = model.CAAS
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
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.OneError(), gc.ErrorMatches, `.*block storage "block" is not supported for container charms`)
}

func (s *ApplicationSuite) TestDeployCAASModelNoOperatorStorage(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.modelConfig = s.modelConfig.Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
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
				},
			},
			NumUnits: 1,
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `.*deploying this Kubernetes application requires a suitable storage class.*`)
}

func (s *ApplicationSuite) TestDeployCAASModelCharmNeedsNoOperatorStorage(c *gc.C) {
	s.modelInfo.Type = model.CAAS
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
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(config.UseDefaults, attrs)).AnyTimes()

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelSidecarCharmNeedsNoOperatorStorage(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.modelConfig = s.modelConfig.Merge(map[string]interface{}{
		"workload-storage": "k8s-storage",
	})
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectCharm(ctrl, &charm.Meta{}, &charm.Manifest{Bases: []charm.Base{{}}}, &charm.Config{})
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultOperatorStorageClass(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.addDefaultK8sModelConfig()
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any(), gomock.Any()).Return(nil)
	s.storagePoolGetter.EXPECT().GetStoragePoolByName(gomock.Any(), "k8s-operator-storage").Return(nil, fmt.Errorf("storage pool not found%w", errors.Hide(storageerrors.PoolNotFoundError)))
	s.registry.EXPECT().StorageProvider(storage.ProviderType("k8s-operator-storage")).Return(nil, errors.NotFoundf(`provider type "k8s-operator-storage"`))

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelWrongOperatorStorageType(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.addDefaultK8sModelConfig()
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.storagePoolGetter.EXPECT().GetStoragePoolByName(gomock.Any(), "k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		provider.RootfsProviderType,
		map[string]interface{}{"foo": "bar"}),
	)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `.*the "k8s-operator-storage" storage pool requires a provider type of "kubernetes", not "rootfs"`)
}

func (s *ApplicationSuite) TestDeployCAASModelInvalidStorage(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.addDefaultK8sModelConfig()
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.storagePoolGetter.EXPECT().GetStoragePoolByName(gomock.Any(), "k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any(), gomock.Any()).Return(errors.NotFoundf("storage class"))

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Storage: map[string]storage.Directive{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `.*storage class not found`)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultStorageClass(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.addDefaultK8sModelConfig()
	ctrl := s.setup(c)
	defer ctrl.Finish()

	ch := s.expectDefaultCharm(ctrl)
	s.backend.EXPECT().Charm(gomock.Any()).Return(ch, nil)
	s.storagePoolGetter.EXPECT().GetStoragePoolByName(gomock.Any(), "k8s-operator-storage").Return(storage.NewConfig(
		"k8s-operator-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.storagePoolGetter.EXPECT().GetStoragePoolByName(gomock.Any(), "k8s-storage").Return(storage.NewConfig(
		"k8s-storage",
		k8sconstants.StorageProviderType,
		map[string]interface{}{"foo": "bar"}),
	)
	s.caasBroker.EXPECT().ValidateStorageClass(gomock.Any(), gomock.Any()).Return(nil)

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local", Base: params.Base{Name: "ubuntu", Channel: "20.04/stable"}},
			NumUnits:        1,
			Storage: map[string]storage.Directive{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestAddUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unitName := "postgresql/99"
	newUnit := s.expectUnit(ctrl, unitName)
	newUnit.EXPECT().AssignWithPolicy(state.AssignNew)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AddUnit(state.AddUnitParams{AttachStorage: []names.StorageTag{}}).Return(newUnit, nil)
	s.backend.EXPECT().Application("postgresql").AnyTimes().Return(app, nil)

	s.networkService.EXPECT().GetAllSpaces(gomock.Any())
	s.machineService.EXPECT().CreateMachine(gomock.Any(), machine.Name("99"))
	s.applicationService.EXPECT().AddUnits(
		gomock.Any(), "postgresql",
		applicationservice.AddUnitArg{UnitName: unitName},
	)

	results, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.AddApplicationUnitsResults{
		Units: []string{"postgresql/99"},
	})
}

func (s *ApplicationSuite) TestAddUnitsCAASModel(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	defer s.setup(c).Finish()

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, gc.ErrorMatches, "adding units to a container-based model not supported")
}

func (s *ApplicationSuite) TestAddUnitsAttachStorage(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unitName := "postgresql/99"
	newUnit := s.expectUnit(ctrl, unitName)
	newUnit.EXPECT().AssignWithPolicy(state.AssignNew)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().AddUnit(state.AddUnitParams{
		AttachStorage: []names.StorageTag{names.NewStorageTag("pgdata/0")},
	}).Return(newUnit, nil)
	s.backend.EXPECT().Application("postgresql").AnyTimes().Return(app, nil)

	s.networkService.EXPECT().GetAllSpaces(gomock.Any())
	s.machineService.EXPECT().CreateMachine(gomock.Any(), machine.Name("99"))
	s.applicationService.EXPECT().AddUnits(
		gomock.Any(), "postgresql",
		applicationservice.AddUnitArg{UnitName: unitName},
	)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
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

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
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

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"volume-0"},
	})
	c.Assert(err, gc.ErrorMatches, `"volume-0" is not a valid storage tag`)
}

func (s *ApplicationSuite) TestDestroyUnitsCAASModel(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	defer s.setup(c).Finish()

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{
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
	s.modelInfo.Type = model.CAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().SetApplicationScale(gomock.Any(), "postgresql", 5)

	results, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
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

func (s *ApplicationSuite) TestScaleApplicationsBlocked(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleChange(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().ChangeApplicationScale(gomock.Any(), "postgresql", 5).Return(7, nil)

	results, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
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
	s.modelInfo.Type = model.CAAS
	defer s.setup(c).Finish()

	results, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
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
	s.modelInfo.Type = model.CAAS
	defer s.setup(c).Finish()

	results, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
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

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{
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

	results, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	results, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	results, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	results, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	results, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{
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
		results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.OneError(), gc.IsNil)
	}
}

func (s *ApplicationSuite) TestConsumeFromExternalController(c *gc.C) {
	defer s.setup(c).Finish()

	controllerUUID := uuid.MustNewUUID().String()

	s.addRemoteApplicationParams.ExternalControllerUUID = controllerUUID

	s.consumeApplicationArgs.Args[0].ControllerInfo = &params.ExternalControllerInfo{
		ControllerTag: names.NewControllerTag(controllerUUID).String(),
		Alias:         "controller-alias",
		CACert:        coretesting.CACert,
		Addrs:         []string{"192.168.1.1:1234"},
	}

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(nil, errors.NotFoundf(`saas application "hosted-mysql"`))
	s.ecService.EXPECT().UpdateExternalController(
		context.Background(),
		crossmodel.ControllerInfo{
			ControllerTag: names.NewControllerTag(controllerUUID),
			Alias:         "controller-alias",
			CACert:        coretesting.CACert,
			Addrs:         []string{"192.168.1.1:1234"},
			ModelUUIDs:    []string{coretesting.ModelTag.Id()},
		},
	).Return(nil)
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)
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

	results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeIncludesSpaceInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.addRemoteApplicationParams.Name = "beirut"

	s.consumeApplicationArgs.Args[0].ApplicationAlias = "beirut"

	s.backend.EXPECT().RemoteApplication("beirut").Return(nil, errors.NotFoundf(`saas application "beirut"`))
	s.backend.EXPECT().AddRemoteApplication(s.addRemoteApplicationParams).Return(nil, nil)

	results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeRemoteAppExistsDifferentSourceModel(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.backend.EXPECT().RemoteApplication("hosted-mysql").Return(s.expectRemoteApplication(ctrl, state.Alive, status.Active), nil)

	s.consumeApplicationArgs.Args[0].ApplicationOfferDetailsV5.SourceModelTag = names.NewModelTag(uuid.MustNewUUID().String()).String()

	results, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(uuid.MustNewUUID().String()).String(),
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

	results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)

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

	results, err := s.api.Consume(context.Background(), s.consumeApplicationArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *ApplicationSuite) TestRemoteRelationBadCIDR(c *gc.C) {
	defer s.setup(c).Finish()

	s.backend.EXPECT().InferEndpoints("wordpress", "hosted-mysql:nope").Return([]state.Endpoint{{
		ApplicationName: "wordpress",
	}, {
		ApplicationName: "hosted-mysql",
	}}, nil)
	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"bad.cidr"}})
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
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"10.10.0.0/16"}})
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
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"0.0.0.0/0"}})
	c.Assert(err, gc.ErrorMatches, `CIDR "0.0.0.0/0" not allowed`)
}

func (s *ApplicationSuite) TestUnsetApplicationConfig(c *gc.C) {
	s.modelInfo.Type = model.CAAS
	ctrl := s.setup(c)
	defer ctrl.Finish()

	schemaFields, defaults, err := application.ConfigSchema()
	c.Assert(err, jc.ErrorIsNil)

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().UpdateApplicationConfig(coreconfig.ConfigAttributes(nil), []string{"trust"}, schemaFields, defaults)
	app.EXPECT().UpdateCharmConfig(charm.Settings{"stringVal": nil})
	s.backend.EXPECT().Application("postgresql").Return(app, nil)

	result, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "postgresql",
			Options:         []string{"trust", "stringVal"},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestBlockUnsetApplicationConfig(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, gc.ErrorMatches, "change blocked")
}

func (s *ApplicationSuite) TestUnsetApplicationConfigPermissionDenied(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("fred"),
	}
	s.modelInfo.Type = model.CAAS
	defer s.setup(c).Finish()

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{
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
	result, err := s.api.ResolveUnitErrors(context.Background(), p)
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
	_, err := s.api.ResolveUnitErrors(context.Background(), p)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestBlockResolveUnitErrors(c *gc.C) {
	s.changeAllowed = errors.New("change blocked")
	defer s.setup(c).Finish()

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
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
	_, err := s.api.ResolveUnitErrors(context.Background(), p)
	c.Assert(err, gc.ErrorMatches, "permission denied")
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
	app.EXPECT().CharmConfig().Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

	bindings := mocks.NewMockBindings(ctrl)
	bindings.EXPECT().MapWithSpaceNames(gomock.Any()).Return(map[string]string{"juju-info": "myspace"}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(bindings, nil).MinTimes(1)
	app.EXPECT().ExposedEndpoints().Return(nil).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Times(2)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(context.Background(), params.Entities{Entities: entities})
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
	app.EXPECT().CharmConfig().Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

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
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Times(3).Return(s.allSpaceInfos, nil)

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(context.Background(), params.Entities{Entities: entities})
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
	app.EXPECT().CharmConfig().Return(nil, errors.Errorf("boom"))
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any())

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(context.Background(), params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestApplicationsInfoBindingsErr(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	app := s.expectDefaultApplication(ctrl)
	app.EXPECT().ApplicationConfig().Return(coreconfig.ConfigAttributes{}, nil).MinTimes(1)
	app.EXPECT().CharmConfig().Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(nil, errors.Errorf("boom")).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any())

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(context.Background(), params.Entities{Entities: entities})
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
	app.EXPECT().CharmConfig().Return(map[string]interface{}{"stringOption": "", "intOption": int(123)}, nil).MinTimes(1)

	bindings := mocks.NewMockBindings(ctrl)
	bindings.EXPECT().MapWithSpaceNames(gomock.Any()).Return(map[string]string{"juju-info": "myspace"}, nil).MinTimes(1)
	app.EXPECT().EndpointBindings().Return(bindings, nil).MinTimes(1)
	app.EXPECT().ExposedEndpoints().Return(map[string]state.ExposedEndpoint{}).MinTimes(1)
	s.backend.EXPECT().Application("postgresql").Return(app, nil).MinTimes(1)

	// wordpress
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), "wordpress").Return("", applicationerrors.ApplicationNotFound)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Times(2)

	entities := []params.Entity{{Tag: "application-postgresql"}, {Tag: "application-wordpress"}, {Tag: "unit-postgresql-0"}}
	result, err := s.api.ApplicationsInfo(context.Background(), params.Entities{Entities: entities})
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
	result, err := s.api.MergeBindings(context.Background(), req)

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
	result, err := s.api.UnitsInfo(context.Background(), params.Entities{Entities: entities})
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
	result, err := s.api.UnitsInfo(context.Background(), params.Entities{Entities: entities})
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

	result, err := s.api.Leader(context.Background(), params.Entity{Tag: names.NewApplicationTag("postgresql").String()})
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
	foo.EXPECT().CharmConfig().Return(map[string]interface{}{
		"title":       "foo",
		"skill-level": 42,
	}, nil)
	s.backend.EXPECT().Application("foo").Return(foo, nil)

	bar := s.expectApplicationWithCharm(ctrl, ch, "bar")
	bar.EXPECT().CharmConfig().Return(map[string]interface{}{
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
	results, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{
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

	results, err := s.api.GetConfig(context.Background(), params.Entities{
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

	result, err := s.api.GetCharmURLOrigin(context.Background(), params.ApplicationGet{ApplicationName: "postgresql"})
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
