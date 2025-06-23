// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication/jwt"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/jwtparser"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
)

type WorkerStateSuite struct {
	workerFixture
}

func TestWorkerStateSuite(t *testing.T) {
	tc.Run(t, &WorkerStateSuite{})
}

func (s *WorkerStateSuite) SetUpTest(c *tc.C) {
	s.workerFixture.SetUpTest(c)
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

func (s *WorkerStateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelService = NewMockModelService(ctrl)

	s.config.ControllerConfigService = s.controllerConfigService
	s.config.ModelService = s.modelService

	return ctrl
}

func (s *WorkerStateSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(
		map[string]any{"controller-uuid": coretesting.ControllerTag.Id()},
		nil,
	)
	s.modelService.EXPECT().ControllerModel(gomock.Any()).Return(model.Model{
		UUID: s.controllerModelUUID,
	}, nil)
	w, err := apiserver.NewWorker(c.Context(), s.config)
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, coreapiserver.ServerConfig{})
	config := args[0].(coreapiserver.ServerConfig)

	c.Assert(config.RegisterIntrospectionHandlers, tc.NotNil)
	config.RegisterIntrospectionHandlers = nil

	c.Assert(config.UpgradeComplete, tc.NotNil)
	config.UpgradeComplete = nil

	c.Assert(config.NewObserver, tc.NotNil)
	config.NewObserver = nil

	c.Assert(config.GetAuditConfig, tc.NotNil)
	// Set the audit config getter to Nil because we don't want to
	// compare it.
	config.GetAuditConfig = nil

	logSinkConfig := coreapiserver.DefaultLogSinkConfig()

	jwtAuthenticator := jwt.NewAuthenticator(&jwtparser.Parser{})

	c.Assert(config, tc.DeepEquals, coreapiserver.ServerConfig{
		LocalMacaroonAuthenticator: s.authenticator,
		Mux:                        s.mux,
		Clock:                      s.clock,
		Tag:                        s.agentConfig.Tag(),
		DataDir:                    s.agentConfig.DataDir(),
		LogDir:                     s.agentConfig.LogDir(),
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
