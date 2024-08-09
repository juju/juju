// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"regexp"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/domain/access"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/internal/docker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	pscontroller "github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type controllerSuite struct {
	statetesting.StateSuite
	servicefactorytesting.ServiceFactorySuite

	controllerConfigAttrs map[string]any

	controller       *controller.ControllerAPI
	resources        *common.Resources
	watcherRegistry  facade.WatcherRegistry
	authorizer       apiservertesting.FakeAuthorizer
	hub              *pubsub.StructuredHub
	context          facadetest.MultiModelContext
	leadershipReader leadership.Reader
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpSuite(c *gc.C) {
	s.StateSuite.SetUpSuite(c)
	s.ServiceFactorySuite.SetUpSuite(c)
}

func (s *controllerSuite) SetUpTest(c *gc.C) {
	if s.controllerConfigAttrs == nil {
		s.controllerConfigAttrs = map[string]any{}
	}
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})
	controllerCfg := testing.FakeControllerConfig()
	for key, value := range s.controllerConfigAttrs {
		controllerCfg[key] = value
	}

	s.StateSuite.ControllerConfig = controllerCfg
	s.StateSuite.SetUpTest(c)
	s.ServiceFactorySuite.ControllerConfig = controllerCfg
	s.ServiceFactorySuite.SetUpTest(c)
	jujujujutesting.SeedDatabase(c, s.ControllerSuite.TxnRunner(), s.ServiceFactoryGetter(c)(s.ControllerModelUUID), controllerCfg)

	s.hub = pubsub.NewStructuredHub(nil)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	s.leadershipReader = noopLeadershipReader{}
	s.context = facadetest.MultiModelContext{ModelContext: facadetest.ModelContext{
		State_:            s.State,
		StatePool_:        s.StatePool,
		Resources_:        s.resources,
		WatcherRegistry_:  s.watcherRegistry,
		Auth_:             s.authorizer,
		Hub_:              s.hub,
		ServiceFactory_:   s.ControllerServiceFactory(c),
		Logger_:           loggertesting.WrapCheckLog(c),
		LeadershipReader_: s.leadershipReader,
	}}
	controller, err := controller.LatestAPI(context.Background(), s.context)
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *controllerSuite) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
	s.ServiceFactorySuite.TearDownTest(c)
}

func (s *controllerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := controller.LatestAPI(context.Background(), facadetest.MultiModelContext{ModelContext: facadetest.ModelContext{
		State_:          s.State,
		Resources_:      s.resources,
		Auth_:           anAuthoriser,
		ServiceFactory_: s.ControllerServiceFactory(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	}})
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestListBlockedModelsNoBlocks(c *gc.C) {
	list, err := s.controller.ListBlockedModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.Models, gc.HasLen, 0)
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.controller.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	controllerConfigService := s.ControllerServiceFactory(c).ControllerConfig()

	cfgFromDB, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestControllerConfigFromNonController(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer func() { _ = st.Close() }()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.Owner}
	controller, err := controller.LatestAPI(
		context.Background(),
		facadetest.MultiModelContext{ModelContext: facadetest.ModelContext{
			State_:          st,
			Resources_:      common.NewResources(),
			Auth_:           authorizer,
			ServiceFactory_: s.ControllerServiceFactory(c),
			Logger_:         loggertesting.WrapCheckLog(c),
		}})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := controller.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	controllerConfigService := s.ControllerServiceFactory(c).ControllerConfig()

	cfgFromDB, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestRemoveBlocks(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer func() { _ = st.Close() }()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.RemoveBlocks(context.Background(), params.RemoveBlocksArgs{All: true})
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *controllerSuite) TestRemoveBlocksNotAll(c *gc.C) {
	err := s.controller.RemoveBlocks(context.Background(), params.RemoveBlocksArgs{})
	c.Assert(err, gc.ErrorMatches, "not supported")
}

func (s *controllerSuite) TestGrantControllerInvalidUserTag(c *gc.C) {
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user@local",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify controller access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyControllerAccessRequest{
			Changes: []params.ModifyControllerAccess{{
				UserTag: testParam.tag,
				Action:  params.GrantControllerAccess,
				Access:  string(permission.SuperuserAccess),
			}}}

		result, err := s.controller.ModifyControllerAccess(context.Background(), args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	}
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	// Check that we don't err out immediately if a model errs.
	results, err := s.controller.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: s.Model.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.controller.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we return successfully if no errors.
	results, err = s.controller.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *controllerSuite) TestConfigSet(c *gc.C) {
	controllerConfigService := s.ControllerServiceFactory(c).ControllerConfig()

	config, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	// Sanity check.
	c.Assert(config.AuditingEnabled(), gc.Equals, false)

	err = s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"auditing-enabled": true,
	}})
	c.Assert(err, jc.ErrorIsNil)

	config, err = controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.AuditingEnabled(), gc.Equals, true)
}

func (s *controllerSuite) TestConfigSetRequiresSuperUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Access: permission.ReadAccess,
	})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endpoint, err := controller.LatestAPI(
		context.Background(),
		facadetest.MultiModelContext{ModelContext: facadetest.ModelContext{
			State_:          s.State,
			Resources_:      s.resources,
			Auth_:           anAuthoriser,
			ServiceFactory_: s.ControllerServiceFactory(c),
			Logger_:         loggertesting.WrapCheckLog(c),
		}})
	c.Assert(err, jc.ErrorIsNil)

	err = endpoint.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"something": 23,
	}})

	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestConfigSetPublishesEvent(c *gc.C) {
	done := make(chan struct{})
	var config corecontroller.Config
	s.hub.Subscribe(pscontroller.ConfigChanged, func(topic string, data pscontroller.ConfigChangedMessage, err error) {
		c.Check(err, jc.ErrorIsNil)
		config = data.Config
		close(done)
	})

	err := s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"features": "foo,bar",
	}})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("no event sent}")
	}

	c.Assert(config.Features().SortedValues(), jc.DeepEquals, []string{"bar", "foo"})
}

func (s *controllerSuite) TestConfigSetCAASImageRepo(c *gc.C) {
	// TODO(dqlite): move this test when ConfigSet CAASImageRepo logic moves.
	controllerConfigService := s.ControllerServiceFactory(c).ControllerConfig()

	config, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.CAASImageRepo(), gc.Equals, "")

	err = s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": "juju-repo.local",
	}})
	c.Assert(err, gc.ErrorMatches, `cannot change caas-image-repo as it is not currently set`)

	err = controllerConfigService.UpdateControllerConfig(
		context.Background(),
		map[string]interface{}{
			"caas-image-repo": "jujusolutions",
		}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": "juju-repo.local",
	}})
	c.Assert(err, gc.ErrorMatches, `cannot change caas-image-repo: repository read-only, only authentication can be updated`)

	err = s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": `{"repository":"jujusolutions","username":"foo","password":"bar"}`,
	}})
	c.Assert(err, gc.ErrorMatches, `cannot change caas-image-repo: unable to add authentication details`)

	err = controllerConfigService.UpdateControllerConfig(
		context.Background(),
		map[string]interface{}{
			"caas-image-repo": `{"repository":"jujusolutions","username":"bar","password":"foo"}`,
		}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.controller.ConfigSet(context.Background(), params.ControllerConfigSet{Config: map[string]interface{}{
		"caas-image-repo": `{"repository":"jujusolutions","username":"foo","password":"bar"}`,
	}})
	c.Assert(err, jc.ErrorIsNil)

	config, err = controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	repoDetails, err := docker.NewImageRepoDetails(config.CAASImageRepo())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repoDetails, gc.DeepEquals, docker.ImageRepoDetails{
		Repository: "jujusolutions",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "foo",
			Password: "bar",
		},
	})
}

func (s *controllerSuite) TestMongoVersion(c *gc.C) {
	result, err := s.controller.MongoVersion(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	var resErr *params.Error
	c.Assert(result.Error, gc.Equals, resErr)
	// We can't guarantee which version of mongo is running, so let's just
	// attempt to match it to a very basic version (major.minor.patch)
	c.Assert(result.Result, gc.Matches, "^([0-9]{1,}).([0-9]{1,}).([0-9]{1,})$")
}

func (s *controllerSuite) TestIdentityProviderURL(c *gc.C) {
	// Preserve default controller config as we will be mutating it just
	// for this test
	defer func(orig map[string]interface{}) {
		s.controllerConfigAttrs = orig
	}(s.controllerConfigAttrs)

	// Our default test configuration does not specify an IdentityURL
	urlRes, err := s.controller.IdentityProviderURL(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urlRes.Result, gc.Equals, "")

	// IdentityURL cannot be changed after bootstrap; we need to spin up
	// another controller with IdentityURL pre-configured
	s.TearDownTest(c)

	expURL := "https://api.jujucharms.com/identity"
	s.controllerConfigAttrs = map[string]any{
		corecontroller.IdentityURL: expURL,
	}

	s.SetUpTest(c)

	urlRes, err = s.controller.IdentityProviderURL(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urlRes.Result, gc.Equals, expURL)
}

func (s *controllerSuite) newSummaryWatcherFacade(c *gc.C, id string) *apiserver.SrvModelSummaryWatcher {
	context := s.context
	context.ID_ = id
	watcher, err := apiserver.NewModelSummaryWatcher(context)
	c.Assert(err, jc.ErrorIsNil)
	return watcher
}

func (s *controllerSuite) TestWatchAllModelSummariesByAdmin(c *gc.C) {
	// TODO(dqlite) - implement me
	c.Skip("watch model summaries to be implemented")
	// Default authorizer is an admin.
	result, err := s.controller.WatchAllModelSummaries(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	watcherAPI := s.newSummaryWatcherFacade(c, result.WatcherID)

	resultC := make(chan params.SummaryWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		c.Assert(result, jc.DeepEquals, params.SummaryWatcherNextResults{
			Models: []params.ModelAbstract{
				{
					UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					Controller: "", // TODO(thumper): add controller name next branch
					Name:       "controller",
					Admins:     []string{"test-admin"},
					Cloud:      "dummy",
					Region:     "dummy-region",
					Status:     "green",
					Messages:   []params.ModelSummaryMessage{},
				}}})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestWatchAllModelSummariesByNonAdmin(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewLocalUserTag("bob"),
	}
	endPoint, err := controller.LatestAPI(
		context.Background(),
		facadetest.MultiModelContext{ModelContext: facadetest.ModelContext{
			State_:          s.State,
			Resources_:      s.resources,
			Auth_:           anAuthoriser,
			ServiceFactory_: s.ControllerServiceFactory(c),
			Logger_:         loggertesting.WrapCheckLog(c),
		}})
	c.Assert(err, jc.ErrorIsNil)

	_, err = endPoint.WatchAllModelSummaries(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) makeBobsModel(c *gc.C) string {
	bob := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bob",
		NoModelUser: true,
	})
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Owner: bob.UserTag(),
		Name:  "bobs-model"})
	uuid := st.ModelUUID()
	st.Close()
	s.WaitForModelWatchersIdle(c, uuid)
	return uuid
}

func (s *controllerSuite) TestWatchModelSummariesByNonAdmin(c *gc.C) {
	// TODO(dqlite) - implement me
	c.Skip("watch model summaries to be implemented")
	s.makeBobsModel(c)

	// Default authorizer is an admin. As a user, admin can't see
	// Bob's model.
	result, err := s.controller.WatchModelSummaries(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	watcherAPI := s.newSummaryWatcherFacade(c, result.WatcherID)

	resultC := make(chan params.SummaryWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		c.Assert(result, jc.DeepEquals, params.SummaryWatcherNextResults{
			Models: []params.ModelAbstract{
				{
					UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					Controller: "", // TODO(thumper): add controller name next branch
					Name:       "controller",
					Admins:     []string{"test-admin"},
					Cloud:      "dummy",
					Region:     "dummy-region",
					Status:     "green",
					Messages:   []params.ModelSummaryMessage{},
				}}})
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}

}

type accessSuite struct {
	statetesting.StateSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	accessService *controller.MockControllerAccessService
	modelService  *controller.MockModelService
}

var _ = gc.Suite(&accessSuite{})

func (s *accessSuite) SetUpSuite(c *gc.C) {
	s.StateSuite.SetUpSuite(c)
}

func (s *accessSuite) SetUpTest(c *gc.C) {
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})
	controllerCfg := testing.FakeControllerConfig()

	s.StateSuite.ControllerConfig = controllerCfg
	s.StateSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

}

func (s *accessSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = controller.NewMockControllerAccessService(ctrl)
	s.modelService = controller.NewMockModelService(ctrl)
	return ctrl
}

func (s *accessSuite) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
}

func (s *accessSuite) controllerAPI(c *gc.C) *controller.ControllerAPI {
	api, err := controller.NewControllerAPI(
		modeltesting.GenModelUUID(c),
		s.State,
		nil,
		s.authorizer,
		nil,
		nil,
		nil,
		loggertesting.WrapCheckLog(c),
		nil,
		nil,
		nil,
		nil,
		nil,
		s.accessService,
		s.modelService,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return api
}

func (s *accessSuite) TestModifyControllerAccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	userName := "test-user"

	external := false
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        testing.ControllerTag.Id(),
			},
		},
		AddUser:  true,
		External: &external,
		ApiUser:  "test-admin",
		Change:   permission.Grant,
		Subject:  userName,
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyControllerAccessRequest{Changes: []params.ModifyControllerAccess{{
		UserTag: names.NewUserTag(userName).String(),
		Action:  params.GrantControllerAccess,
		Access:  string(permission.SuperuserAccess),
	}}}

	result, err := s.controllerAPI(c).ModifyControllerAccess(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *accessSuite) TestGetControllerAccessPermissions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	userName := "test-user"
	userTag := names.NewUserTag(userName)
	differentUser := "different-test-user"

	target := permission.AccessSpec{
		Access: permission.SuperuserAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        testing.ControllerTag.Id(),
		},
	}
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, target.Target).Return(permission.SuperuserAccess, nil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	req := params.Entities{
		Entities: []params.Entity{{Tag: userTag.String()}, {Tag: names.NewUserTag(differentUser).String()}},
	}
	results, err := s.controllerAPI(c).GetControllerAccess(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(*results.Results[0].Result, jc.DeepEquals, params.UserAccess{
		Access:  "superuser",
		UserTag: userTag.String(),
	})
	c.Assert(*results.Results[1].Error, gc.DeepEquals, params.Error{
		Message: "permission denied", Code: "unauthorized access",
	})
}

func (s *accessSuite) TestAllModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Controller model is already in state
	controllerModelID := coremodel.UUID(s.State.ControllerModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), controllerModelID).Return(coremodel.Model{
		Name:      "controller",
		UUID:      controllerModelID,
		ModelType: coremodel.IAAS,
		OwnerName: "test-admin",
	}, nil)

	admin := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	ownedSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "owned", Owner: admin.UserTag()})
	ownedModelID := coremodel.UUID(ownedSt.ModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), ownedModelID).Return(coremodel.Model{
		Name:      "owned",
		UUID:      ownedModelID,
		ModelType: coremodel.IAAS,
		OwnerName: "foobar",
	}, nil)
	ownedSt.Close()

	remoteUserTag := names.NewUserTag("user@remote")
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "user", Owner: remoteUserTag})
	defer func() { _ = st.Close() }()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	userModelID := coremodel.UUID(model.UUID())
	s.modelService.EXPECT().Model(gomock.Any(), userModelID).Return(coremodel.Model{
		Name:      "user",
		UUID:      userModelID,
		ModelType: coremodel.IAAS,
		OwnerName: "user@remote",
	}, nil)

	model.AddUser(
		state.UserAccessSpec{
			User:        admin.UserTag(),
			CreatedBy:   remoteUserTag,
			DisplayName: "Foo Bar",
			Access:      permission.WriteAccess})

	noAccessSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "no-access", Owner: remoteUserTag})
	noAccessModelID := coremodel.UUID(noAccessSt.ModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), noAccessModelID).Return(coremodel.Model{
		Name:      "no-access",
		UUID:      noAccessModelID,
		ModelType: coremodel.IAAS,
		OwnerName: "user@remote",
	}, nil)
	noAccessSt.Close()

	s.accessService.EXPECT().LastModelLogin(gomock.Any(), "test-admin", gomock.Any()).Times(4)

	response, err := s.controllerAPI(c).AllModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	// The results are sorted.
	expected := []string{"controller", "no-access", "owned", "user"}
	var obtained []string
	for _, userModel := range response.UserModels {
		c.Assert(userModel.Type, gc.Equals, "iaas")
		obtained = append(obtained, userModel.Name)
		stateModel, ph, err := s.StatePool.GetModel(userModel.UUID)
		c.Assert(err, jc.ErrorIsNil)
		defer ph.Release()
		s.checkModelMatches(c, userModel.Model, stateModel)
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *accessSuite) checkModelMatches(c *gc.C, model params.Model, expected *state.Model) {
	c.Check(model.Name, gc.Equals, expected.Name())
	c.Check(model.UUID, gc.Equals, expected.UUID())
	c.Check(model.OwnerTag, gc.Equals, expected.Owner().String())
}

type noopLeadershipReader struct {
	leadership.Reader
}

func (noopLeadershipReader) Leaders() (map[string]string, error) {
	return make(map[string]string), nil
}
