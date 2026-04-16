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

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	auth "github.com/juju/juju/internal/auth"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujujujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/sockets"
)

type workerSuite struct {
	accessService  *MockAccessService
	tracingService *MockTracingService

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

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = NewMockAccessService(ctrl)
	s.tracingService = NewMockTracingService(ctrl)

	c.Cleanup(func() {
		s.accessService = nil
		s.tracingService = nil
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
