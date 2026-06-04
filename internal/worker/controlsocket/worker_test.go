// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	auth "github.com/juju/juju/internal/auth"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujujujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/sockets"
)

type workerSuite struct {
	accessService      *MockAccessService
	tracingService     *MockTracingService
	loggingService     *MockLoggingService
	objectStoreService *MockControllerObjectStoreService

	controllerModelID permission.ID
	metricsUserName   coreuser.Name
}

func TestWorkerSuite(t *testing.T) {
	goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.metricsUserName = usertesting.GenNewName(c, "juju-metrics-r0")
	s.controllerModelID = permission.ID{
		ObjectType: permission.Model,
		Key:        jujujujutesting.ModelTag.Id(),
	}
}

func (s *workerSuite) TestConfigValidateSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)
}

func (s *workerSuite) TestConfigValidateNilAccessService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.AccessService = nil
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*nil AccessService.*")
}

func (s *workerSuite) TestConfigValidateNilObjectStoreService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.ObjectStoreService = nil
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*nil ObjectStoreService.*")
}

func (s *workerSuite) TestConfigValidateEmptyControllerModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.ControllerModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*empty ControllerModelUUID.*")
}

func (s *workerSuite) TestConfigValidateEmptySocketName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.SocketName = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*empty SocketName.*")
}

func (s *workerSuite) TestConfigValidateNilNewSocketListener(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.NewSocketListener = nil
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*nil NewSocketListener.*")
}

func (s *workerSuite) TestConfigValidateNilLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newValidConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorMatches, ".*nil Logger.*")
}

func (s *workerSuite) TestNewWorkerInvalidConfig(c *tc.C) {
	_, err := NewWorker(Config{})
	c.Assert(err, tc.ErrorMatches, ".*nil AccessService.*")
}

func (s *workerSuite) TestWorkerKillAndWait(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestMetricsUsersAddInvalidMethod(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestMetricsUsersAddMissingBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddInvalidBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       "username foo, password bar",
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddMissingUsername(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*missing username.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddUsernameMissingPrefix(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"foo","password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        s.metricsUserName,
		DisplayName: "juju-metrics-r0",
		Password:    new(auth.NewPassword("bar")),
		CreatorUUID: coreuser.UUID("deadbeef"),
		Permission: permission.AccessSpec{
			Target: s.controllerModelID,
			Access: permission.ReadAccess,
		},
	}).Return(coreuser.UUID("foobar"), nil, nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusOK,
		response:   `.*created user \\\"juju-metrics-r0\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        s.metricsUserName,
		DisplayName: "juju-metrics-r0",
		Password:    new(auth.NewPassword("bar")),
		CreatorUUID: coreuser.UUID("deadbeef"),
		Permission: permission.AccessSpec{
			Target: s.controllerModelID,
			Access: permission.ReadAccess,
		},
	}).Return(coreuser.UUID("foobar"), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{
		CreatorName: usertesting.GenNewName(c, "not-you"),
	}, nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusConflict,
		response:   `.*user .*\(created by \\\"not-you\\\"\).*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExistsButDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        s.metricsUserName,
		DisplayName: "juju-metrics-r0",
		Password:    new(auth.NewPassword("bar")),
		CreatorUUID: coreuser.UUID("deadbeef"),
		Permission: permission.AccessSpec{
			Target: s.controllerModelID,
			Access: permission.ReadAccess,
		},
	}).Return(coreuser.UUID("foobar"), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{
		CreatorName: usertesting.GenNewName(c, "not-you"),
		Disabled:    true,
	}, nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusForbidden,
		response:   ".*user .* is disabled.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExistsButWrongPermissions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        s.metricsUserName,
		DisplayName: "juju-metrics-r0",
		Password:    new(auth.NewPassword("bar")),
		CreatorUUID: coreuser.UUID("deadbeef"),
		Permission: permission.AccessSpec{
			Target: s.controllerModelID,
			Access: permission.ReadAccess,
		},
	}).Return(coreuser.UUID("foobar"), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{
		CreatorName: usertesting.GenNewName(c, userCreator),
	}, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), s.metricsUserName, s.controllerModelID).Return(
		permission.WriteAccess, nil,
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusNotFound,
		response:   ".*unexpected permission for user .*",
	})
}

func (s *workerSuite) TestMetricsUsersAddIdempotent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        s.metricsUserName,
		DisplayName: "juju-metrics-r0",
		Password:    new(auth.NewPassword("bar")),
		CreatorUUID: coreuser.UUID("deadbeef"),
		Permission: permission.AccessSpec{
			Target: s.controllerModelID,
			Access: permission.ReadAccess,
		},
	}).Return(coreuser.UUID("foobar"), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{
		CreatorName: usertesting.GenNewName(c, userCreator),
	}, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), s.metricsUserName, s.controllerModelID).Return(
		permission.ReadAccess, nil,
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*created user \\\"juju-metrics-r0\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddGetCreatorUserError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{}, errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*retrieving creator user.*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddUserServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), gomock.Any()).Return(coreuser.UUID(""), nil, errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*creating user.*boom.*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExistsGetByAuthError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), gomock.Any()).Return(coreuser.UUID(""), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{}, errors.New("auth boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*retrieving existing user.*auth boom.*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExistsReadAccessLevelError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, userCreator)).Return(coreuser.User{
		UUID: coreuser.UUID("deadbeef"),
	}, nil)
	s.accessService.EXPECT().AddUser(gomock.Any(), gomock.Any()).Return(coreuser.UUID(""), nil, usererrors.UserAlreadyExists)
	s.accessService.EXPECT().GetUserByAuth(gomock.Any(), s.metricsUserName, auth.NewPassword("bar")).Return(coreuser.User{
		CreatorName: usertesting.GenNewName(c, userCreator),
	}, nil)
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), s.metricsUserName, s.controllerModelID).Return(
		permission.NoAccess, errors.New("access boom"),
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*retrieving existing user.*access boom.*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveInvalidMethod(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveUsernameMissingPrefix(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), s.metricsUserName).Return(coreuser.User{
		UUID:        coreuser.UUID("deadbeef"),
		CreatorName: usertesting.GenNewName(c, userCreator),
	}, nil)
	s.accessService.EXPECT().RemoveUser(gomock.Any(), s.metricsUserName).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK,
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveForbidden(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), s.metricsUserName).Return(coreuser.User{
		UUID:        coreuser.UUID("deadbeef"),
		Name:        s.metricsUserName,
		CreatorName: usertesting.GenNewName(c, "not-you"),
	}, nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusForbidden,
		response:   `.*cannot remove user \\\"juju-metrics-r0\\\" created by \\\"not-you\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), s.metricsUserName).Return(coreuser.User{
		UUID:        coreuser.UUID("deadbeef"),
		Name:        s.metricsUserName,
		CreatorName: usertesting.GenNewName(c, "not-you"),
	}, usererrors.UserNotFound)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveGetUserByNameError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), s.metricsUserName).Return(coreuser.User{}, errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusInternalServerError,
		response:   `.*boom.*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), s.metricsUserName).Return(coreuser.User{
		UUID:        coreuser.UUID("deadbeef"),
		CreatorName: usertesting.GenNewName(c, userCreator),
	}, nil)
	s.accessService.EXPECT().RemoveUser(gomock.Any(), s.metricsUserName).Return(errors.New("remove boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusInternalServerError,
		response:   `.*remove boom.*`,
	})
}

func (s *workerSuite) TestCharmTracingConfigInvalidMethod(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/charm-tracing-config",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestCharmTracingConfigMissingBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/charm-tracing-config",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestCharmTracingConfigInvalidBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/charm-tracing-config",
		body:       "http_endpoint=abc",
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *workerSuite) TestCharmTracingConfigUnsupportedContentType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:      http.MethodPost,
		endpoint:    "/charm-tracing-config",
		body:        `{"http_endpoint":"http://localhost:4318"}`,
		contentType: "text/plain",
		statusCode:  http.StatusUnsupportedMediaType,
		response:    ".*request Content-Type must be application/json.*",
	})
}

func (s *workerSuite) TestCharmTracingConfigMissingContentType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:          http.MethodPost,
		endpoint:        "/charm-tracing-config",
		body:            `{"http_endpoint":"http://localhost:4318"}`,
		omitContentType: true,
		statusCode:      http.StatusUnsupportedMediaType,
		response:        ".*request Content-Type must be application/json.*",
	})
}

func (s *workerSuite) TestCharmTracingConfigPayloadTooLarge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/charm-tracing-config",
		body:       `{"ca_cert":"` + strings.Repeat("x", maxPayloadBytes+1) + `"}`,
		statusCode: http.StatusRequestEntityTooLarge,
		response:   ".*request body must not exceed .* bytes.*",
	})
}

func (s *workerSuite) TestCharmTracingConfigSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracingService.EXPECT().SetCharmTracingConfig(gomock.Any(), tracingservice.CharmTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "ca-data",
	}).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/charm-tracing-config",
		body:       `{"http_endpoint":"http://localhost:4318","grpc_endpoint":"localhost:4317","ca_cert":"ca-data"}`,
		statusCode: http.StatusOK,
		response:   `.*updated charm tracing config.*`,
	})
}

func (s *workerSuite) TestCharmTracingConfigServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracingService.EXPECT().SetCharmTracingConfig(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/charm-tracing-config",
		body:       `{"http_endpoint":"http://localhost:4318"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*boom.*`,
	})
}

func (s *workerSuite) TestWorkloadTracingConfigInvalidMethod(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/workload-tracing-config",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestWorkloadTracingConfigMissingBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/workload-tracing-config",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestWorkloadTracingConfigInvalidBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/workload-tracing-config",
		body:       "http_endpoint=abc",
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *workerSuite) TestWorkloadTracingConfigUnsupportedContentType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:      http.MethodPost,
		endpoint:    "/workload-tracing-config",
		body:        `{"http_endpoint":"http://localhost:4318"}`,
		contentType: "text/plain",
		statusCode:  http.StatusUnsupportedMediaType,
		response:    ".*request Content-Type must be application/json.*",
	})
}

func (s *workerSuite) TestWorkloadTracingConfigMissingContentType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:          http.MethodPost,
		endpoint:        "/workload-tracing-config",
		body:            `{"http_endpoint":"http://localhost:4318"}`,
		omitContentType: true,
		statusCode:      http.StatusUnsupportedMediaType,
		response:        ".*request Content-Type must be application/json.*",
	})
}

func (s *workerSuite) TestWorkloadTracingConfigPayloadTooLarge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/workload-tracing-config",
		body:       `{"ca_cert":"` + strings.Repeat("x", maxPayloadBytes+1) + `"}`,
		statusCode: http.StatusRequestEntityTooLarge,
		response:   ".*request body must not exceed .* bytes.*",
	})
}

func (s *workerSuite) TestWorkloadTracingConfigSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracingService.EXPECT().SetWorkloadTracingConfig(gomock.Any(), tracingservice.WorkloadTracingConfig{
		HTTPEndpoint:  "http://localhost:4318",
		GRPCEndpoint:  "localhost:4317",
		CACertificate: "ca-data",
	}).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/workload-tracing-config",
		body:       `{"http_endpoint":"http://localhost:4318","grpc_endpoint":"localhost:4317","ca_cert":"ca-data"}`,
		statusCode: http.StatusOK,
		response:   `.*updated workload tracing config.*`,
	})
}

func (s *workerSuite) TestWorkloadTracingConfigSuccessWithOpenTelemetryOptions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openTelemetryStackTraces := new(bool)
	*openTelemetryStackTraces = true
	openTelemetrySampleRatio := new(float64)
	*openTelemetrySampleRatio = 0.5
	openTelemetryTailSamplingThreshold := new(string)
	*openTelemetryTailSamplingThreshold = "250ms"

	s.tracingService.EXPECT().SetWorkloadTracingConfig(gomock.Any(), tracingservice.WorkloadTracingConfig{
		HTTPEndpoint:                       "http://localhost:4318",
		GRPCEndpoint:                       "localhost:4317",
		CACertificate:                      "ca-data",
		OpenTelemetryStackTraces:           openTelemetryStackTraces,
		OpenTelemetrySampleRatio:           openTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold: openTelemetryTailSamplingThreshold,
	}).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:   http.MethodPost,
		endpoint: "/workload-tracing-config",
		body: `{"http_endpoint":"http://localhost:4318","grpc_endpoint":"localhost:4317","ca_cert":"ca-data",` +
			`"open_telemetry_stack_traces":true,"open_telemetry_sample_ratio":0.5,` +
			`"open_telemetry_tail_sampling_threshold":"250ms"}`,
		statusCode: http.StatusOK,
		response:   `.*updated workload tracing config.*`,
	})
}

func (s *workerSuite) TestWorkloadTracingConfigServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracingService.EXPECT().SetWorkloadTracingConfig(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/workload-tracing-config",
		body:       `{"http_endpoint":"http://localhost:4318"}`,
		statusCode: http.StatusInternalServerError,
		response:   `.*boom.*`,
	})
}

func (s *workerSuite) TestAddS3CredentialsInvalidMethod(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/s3-credentials",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestAddS3CredentialsMissingBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsPayloadTooLarge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		body:       strings.Repeat("a", maxPayloadBytes+1),
		statusCode: http.StatusRequestEntityTooLarge,
		response:   ".*must not exceed.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsInvalidJSON(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		body:       `{"Endpoint":`,
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreService.EXPECT().TransitionBackendToS3(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		body:       `{"endpoint":"https://example.com","access_key":"foo","secret_key":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   ".*saving S3 credentials.*boom.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreService.EXPECT().TransitionBackendToS3(gomock.Any(), domainobjectstore.S3Credentials{
		Endpoint:  "https://example.com",
		AccessKey: "foo",
		SecretKey: "bar",
	}).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		body:       `{"endpoint":"https://example.com","access_key":"foo","secret_key":"bar"}`,
		statusCode: http.StatusOK,
		response:   ".*updated S3 credentials.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreService.EXPECT().TransitionBackendToS3(gomock.Any(), gomock.Any()).Return(
		coreerrors.NotValid,
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/s3-credentials",
		body:       `{"endpoint":"https://example.com","access_key":"foo","secret_key":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*invalid S3 credentials.*",
	})
}

func (s *workerSuite) TestRemoveS3CredentialsNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/s3-credentials",
		statusCode: http.StatusNotImplemented,
		response:   ".*removing s3 credentials is not supported.*",
	})
}

func (s *workerSuite) TestAddS3CredentialsUnsupportedContentType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:      http.MethodPost,
		endpoint:    "/s3-credentials",
		body:        `{"endpoint":"https://example.com","access_key":"foo","secret_key":"bar"}`,
		contentType: "text/plain",
		statusCode:  http.StatusUnsupportedMediaType,
		response:    ".*request Content-Type must be application/json.*",
	})
}

func (s *workerSuite) TestSetLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.loggingService.EXPECT().SetLokiEndpoint(gomock.Any(), "http://loki:3100/loki/api/v1/push").Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/loki-endpoint",
		body:       `{"url":"http://loki:3100/loki/api/v1/push"}`,
		statusCode: http.StatusOK,
		response:   ".*updated loki endpoint.*",
	})
}

func (s *workerSuite) TestSetLokiEndpointEmptyURL(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.loggingService.EXPECT().SetLokiEndpoint(gomock.Any(), "").Return(
		internalerrors.Errorf("empty loki endpoint").Add(coreerrors.NotValid),
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/loki-endpoint",
		body:       `{"url":""}`,
		statusCode: http.StatusBadRequest,
		response:   ".*invalid loki endpoint.*",
	})
}

func (s *workerSuite) TestSetLokiEndpointMissingBody(c *tc.C) {
	defer s.setupMocks(c).Finish()

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/loki-endpoint",
		body:       "",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestRemoveLokiEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.loggingService.EXPECT().DeleteLokiEndpoint(gomock.Any()).Return(nil)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/loki-endpoint",
		statusCode: http.StatusOK,
		response:   ".*removed loki endpoint.*",
	})
}

func (s *workerSuite) TestRemoveLokiEndpointError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.loggingService.EXPECT().DeleteLokiEndpoint(gomock.Any()).Return(
		internalerrors.New("database error"),
	)

	socket := s.newSocket(c)

	w := s.newWorker(c, socket)
	defer workertest.CleanKill(c, w)

	s.runHandlerTest(c, socket, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/loki-endpoint",
		statusCode: http.StatusInternalServerError,
		response:   ".*removing loki endpoint.*",
	})
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = NewMockAccessService(ctrl)
	s.tracingService = NewMockTracingService(ctrl)
	s.loggingService = NewMockLoggingService(ctrl)
	s.objectStoreService = NewMockControllerObjectStoreService(ctrl)

	c.Cleanup(func() {
		s.accessService = nil
		s.tracingService = nil
		s.loggingService = nil
		s.objectStoreService = nil
	})

	return ctrl
}

type handlerTest struct {
	// Request
	method          string
	endpoint        string
	body            string
	contentType     string
	omitContentType bool

	// Response
	statusCode int
	response   string // response body
	ignoreBody bool   // if true, test will not read the request body
}

func (s *workerSuite) newValidConfig(c *tc.C) Config {
	return Config{
		AccessService:       s.accessService,
		TracingService:      s.tracingService,
		LoggingService:      s.loggingService,
		ObjectStoreService:  s.objectStoreService,
		Logger:              loggertesting.WrapCheckLog(c),
		SocketName:          "/tmp/test.socket",
		NewSocketListener:   NewSocketListener,
		ControllerModelUUID: model.UUID(jujujujutesting.ModelTag.Id()),
	}
}

func (s *workerSuite) newSocket(c *tc.C) string {
	// We don't need to clean up the socket file because it's created in a
	// temporary directory that will be removed after the test.
	tmpDir := c.MkDir()
	return path.Join(tmpDir, "test.socket")
}

func (s *workerSuite) newWorker(c *tc.C, socket string) *Worker {
	w, err := NewWorker(Config{
		AccessService:       s.accessService,
		TracingService:      s.tracingService,
		LoggingService:      s.loggingService,
		ObjectStoreService:  s.objectStoreService,
		Logger:              loggertesting.WrapCheckLog(c),
		SocketName:          socket,
		NewSocketListener:   NewSocketListener,
		ControllerModelUUID: model.UUID(jujujujutesting.ModelTag.Id()),
	})
	c.Assert(err, tc.ErrorIsNil)

	return w.(*Worker)
}

func (s *workerSuite) runHandlerTest(c *tc.C, socket string, test handlerTest) {
	serverURL := "http://localhost:8080"
	req, err := http.NewRequest(
		test.method,
		serverURL+test.endpoint,
		strings.NewReader(test.body),
	)
	c.Assert(err, tc.ErrorIsNil)
	if !test.omitContentType {
		contentType := test.contentType
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	// Check server is up
	resp, err := client(socket).Do(req)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, test.statusCode)

	if test.ignoreBody {
		return
	}
	data, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	err = resp.Body.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Response should be valid JSON
	c.Check(resp.Header.Get("Content-Type"), tc.Equals, "application/json")
	err = json.Unmarshal(data, new(any))
	c.Assert(err, tc.ErrorIsNil)
	if test.response != "" {
		c.Check(string(data), tc.Matches, test.response)
	}
}

// Return an *http.Client with custom transport that allows it to connect to
// the given Unix socket.
func client(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (conn net.Conn, err error) {
				return sockets.Dialer(sockets.Socket{
					Network: "unix",
					Address: socketPath,
				})
			},
		},
	}
}
