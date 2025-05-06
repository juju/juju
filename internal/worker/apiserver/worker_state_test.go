// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"

	"github.com/juju/collections/set"
	mgotesting "github.com/juju/mgo/v3/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication/jwt"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/jwtparser"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
	statetesting "github.com/juju/juju/state/testing"
)

type WorkerStateSuite struct {
	workerFixture
	statetesting.StateSuite
}

var _ = gc.Suite(&WorkerStateSuite{})

func (s *WorkerStateSuite) SetUpSuite(c *gc.C) {
	s.workerFixture.SetUpSuite(c)
	mgotesting.MgoServer.EnableReplicaSet = true
	err := mgotesting.MgoServer.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.workerFixture.AddCleanup(func(*gc.C) { mgotesting.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *WorkerStateSuite) TearDownSuite(c *gc.C) {
	s.StateSuite.TearDownSuite(c)
	s.workerFixture.TearDownSuite(c)
}

func (s *WorkerStateSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
	s.config.StatePool = s.StatePool
	s.config.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled:        true,
			CaptureAPIArgs: true,
			MaxSizeMB:      200,
			MaxBackups:     5,
			ExcludeMethods: set.NewStrings("Exclude.This"),
			Target:         &apitesting.FakeAuditLog{},
		}
	}
}

func (s *WorkerStateSuite) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
	s.workerFixture.TearDownTest(c)
}

func (s *WorkerStateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelService = NewMockModelService(ctrl)

	s.config.ControllerConfigService = s.controllerConfigService
	s.config.ModelService = s.modelService

	return ctrl
}

func (s *WorkerStateSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(
		map[string]any{"controller-uuid": coretesting.ControllerTag.Id()},
		nil,
	)
	s.modelService.EXPECT().ControllerModel(gomock.Any()).Return(model.Model{
		UUID: s.controllerModelUUID,
	}, nil)
	w, err := apiserver.NewWorker(context.Background(), s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The server is started some time after the worker
	// starts, not necessarily as soon as NewWorker returns.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 0 {
			continue
		}
		break
	}
	if !s.stub.CheckCallNames(c, "NewServer") {
		return
	}
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, coreapiserver.ServerConfig{})
	config := args[0].(coreapiserver.ServerConfig)

	c.Assert(config.RegisterIntrospectionHandlers, gc.NotNil)
	config.RegisterIntrospectionHandlers = nil

	c.Assert(config.UpgradeComplete, gc.NotNil)
	config.UpgradeComplete = nil

	c.Assert(config.NewObserver, gc.NotNil)
	config.NewObserver = nil

	c.Assert(config.GetAuditConfig, gc.NotNil)
	// Set the audit config getter to Nil because we don't want to
	// compare it.
	config.GetAuditConfig = nil

	logSinkConfig := coreapiserver.DefaultLogSinkConfig()

	jwtAuthenticator := jwt.NewAuthenticator(&jwtparser.Parser{})

	c.Assert(config, jc.DeepEquals, coreapiserver.ServerConfig{
		StatePool:                  s.StatePool,
		LocalMacaroonAuthenticator: s.authenticator,
		Mux:                        s.mux,
		Clock:                      s.clock,
		Tag:                        s.agentConfig.Tag(),
		DataDir:                    s.agentConfig.DataDir(),
		LogDir:                     s.agentConfig.LogDir(),
		Hub:                        &s.hub,
		PublicDNSName:              "",
		AllowModelAccess:           false,
		LogSinkConfig:              &logSinkConfig,
		LeaseManager:               s.leaseManager,
		MetricsCollector:           s.metricsCollector,
		LogSink:                    s.logSink,
		CharmhubHTTPClient:         s.charmhubHTTPClient,
		DBGetter:                   s.dbGetter,
		DBDeleter:                  s.dbDeleter,
		DomainServicesGetter:       s.domainServicesGetter,
		ControllerConfigService:    s.controllerConfigService,
		TracerGetter:               s.tracerGetter,
		ObjectStoreGetter:          s.objectStoreGetter,
		ControllerUUID:             s.controllerUUID,
		ControllerModelUUID:        s.controllerModelUUID,
		JWTAuthenticator:           jwtAuthenticator,
	})
}
