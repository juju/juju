// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	state  *fakeState
	logger Logger
}

var _ = gc.Suite(&workerSuite{})

type handlerTest struct {
	// Request
	method   string
	endpoint string
	body     string
	// Response
	statusCode int
	response   string // response body
	ignoreBody bool   // if true, test will not read the request body
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.state = &fakeState{}
	s.logger = loggo.GetLogger(c.TestName())
}

func (s *workerSuite) runHandlerTest(c *gc.C, test handlerTest) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	_, err := NewWorker(Config{
		State:             s.state,
		Logger:            s.logger,
		SocketName:        socket,
		NewSocketListener: NewSocketListener,
	})
	c.Assert(err, jc.ErrorIsNil)

	serverURL := "http://localhost:8080"
	req, err := http.NewRequest(
		test.method,
		serverURL+test.endpoint,
		strings.NewReader(test.body),
	)
	c.Assert(err, jc.ErrorIsNil)

	resp, err := client(socket).Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, test.statusCode)

	if test.ignoreBody {
		return
	}
	data, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	err = resp.Body.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Response should be valid JSON
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, "application/json")
	err = json.Unmarshal(data, &struct{}{})
	c.Assert(err, jc.ErrorIsNil)
	if test.response != "" {
		c.Check(string(data), gc.Matches, test.response)
	}
}

func (s *workerSuite) assertState(c *gc.C, users []fakeUser) {
	c.Assert(len(s.state.users), gc.Equals, len(users))

	for _, expected := range users {
		actual, ok := s.state.users[expected.name]
		c.Assert(ok, gc.Equals, true)
		c.Check(actual.creator, gc.Equals, expected.creator)
		c.Check(actual.password, gc.Equals, expected.password)
	}
}

func (s *workerSuite) TestMetricsUsersAddInvalidMethod(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestMetricsUsersAddMissingBody(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddInvalidBody(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       "username foo, password bar",
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddMissingUsername(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*missing username.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddMissingPassword(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*empty password.*",
	})
}

func (s *workerSuite) TestMetricsUsersAddUsernameMissingPrefix(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"foo","password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersAddSuccess(c *gc.C) {
	s.state = newFakeState(nil)
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusOK,
		response:   `.*created user \\\"juju-metrics-r0\\\".*`,
	})
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: "controller@juju"},
	})
}

func (s *workerSuite) TestMetricsUsersAddAlreadyExists(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: "not-you"},
	})
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusConflict,
		response:   ".*user .* already exists.*",
	})
	// Nothing should have changed.
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: "not-you"},
	})
}

func (s *workerSuite) TestMetricsUsersAddDifferentPassword(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "foo", creator: userCreator},
	})
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusConflict,
		response:   `.*user \\\"juju-metrics-r0\\\" already exists.*`,
	})
	// Nothing should have changed.
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "foo", creator: userCreator},
	})
}

func (s *workerSuite) TestMetricsUsersAddAddErr(c *gc.C) {
	s.state = newFakeState(nil)
	s.state.addErr = fmt.Errorf("spanner in the works")

	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   ".*spanner in the works.*",
	})
	// Nothing should have changed.
	s.assertState(c, nil)
}

func (s *workerSuite) TestMetricsUsersAddIdempotent(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: userCreator},
	})
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*created user \\\"juju-metrics-r0\\\".*`,
	})
	// Nothing should have changed.
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: userCreator},
	})
}

func (s *workerSuite) TestMetricsUsersAddFailed(c *gc.C) {
	s.state = newFakeState(nil)
	s.state.model.err = fmt.Errorf("spanner in the works")

	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   ".*spanner in the works.*",
	})
	s.assertState(c, nil)
}

func (s *workerSuite) TestMetricsUsersRemoveInvalidMethod(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveUsernameMissingPrefix(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *workerSuite) TestMetricsUsersRemoveSuccess(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: "controller@juju"},
	})
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK,
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
	s.assertState(c, nil)
}

func (s *workerSuite) TestMetricsUsersRemoveForbidden(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "foo", creator: "not-you"},
	})
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusForbidden,
		response:   `.*cannot remove user \\\"juju-metrics-r0\\\" created by \\\"not-you\\\".*`,
	})
	// Nothing should have changed.
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "foo", creator: "not-you"},
	})
}

func (s *workerSuite) TestMetricsUsersRemoveNotFound(c *gc.C) {
	s.state = newFakeState(nil)
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
	s.assertState(c, nil)
}

func (s *workerSuite) TestMetricsUsersRemoveIdempotent(c *gc.C) {
	s.state = newFakeState(nil)
	s.state.userErr = stateerrors.NewDeletedUserError("juju-metrics-r0")

	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
	// Nothing should have changed.
	s.assertState(c, nil)
}

func (s *workerSuite) TestMetricsUsersRemoveFailed(c *gc.C) {
	s.state = newFakeState([]fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: userCreator},
	})
	s.state.removeErr = fmt.Errorf("spanner in the works")

	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		body:       `{"username":"juju-metrics-r0","password":"bar"}`,
		statusCode: http.StatusInternalServerError,
		response:   ".*spanner in the works.*",
	})
	// Nothing should have changed.
	s.assertState(c, []fakeUser{
		{name: "juju-metrics-r0", password: "bar", creator: userCreator},
	})
}

type fakeState struct {
	users map[string]fakeUser
	model *fakeModel

	userErr, addErr, removeErr error
}

func newFakeState(users []fakeUser) *fakeState {
	s := &fakeState{
		users: make(map[string]fakeUser, len(users)),
	}
	for _, user := range users {
		s.users[user.name] = user
	}
	s.model = &fakeModel{nil}
	return s
}

func (s *fakeState) User(tag names.UserTag) (user, error) {
	if s.userErr != nil {
		return nil, s.userErr
	}

	username := tag.Name()
	u, ok := s.users[username]
	if !ok {
		return nil, errors.UserNotFoundf("user %q", username)
	}
	return u, nil
}

func (s *fakeState) AddUser(name, displayName, password, creator string) (user, error) {
	if s.addErr != nil {
		return nil, s.addErr
	}

	if _, ok := s.users[name]; ok {
		// The real state code doesn't return the user if it already exists, it
		// returns a typed nil value.
		return (*fakeUser)(nil), errors.AlreadyExistsf("user %q", name)
	}

	u := fakeUser{name, displayName, password, creator}
	s.users[name] = u
	return u, nil
}

func (s *fakeState) RemoveUser(tag names.UserTag) error {
	if s.removeErr != nil {
		return s.removeErr
	}

	username := tag.Name()
	if _, ok := s.users[username]; !ok {
		return errors.UserNotFoundf("user %q", username)
	}

	delete(s.users, username)
	return nil
}

func (s *fakeState) Model() (model, error) {
	return s.model, nil
}

type fakeUser struct {
	name, displayName, password, creator string
}

func (u fakeUser) Name() string {
	return u.name
}

func (u fakeUser) CreatedBy() string {
	return u.creator
}

func (u fakeUser) UserTag() names.UserTag {
	return names.NewUserTag(u.name)
}

func (u fakeUser) PasswordValid(s string) bool {
	return s == u.password
}

type fakeModel struct {
	err error
}

func (m *fakeModel) AddUser(_ state.UserAccessSpec) (permission.UserAccess, error) {
	return permission.UserAccess{}, m.err
}

// Return an *http.Client with custom transport that allows it to connect to
// the given Unix socket.
func client(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (conn net.Conn, err error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}

type fakeLogger struct {
	entries []logEntry
}

type logEntry struct{ level, msg string }

func (f *fakeLogger) write(level string, format string, args ...any) {
	f.entries = append(f.entries, logEntry{level, fmt.Sprintf(format, args...)})
}

func (f *fakeLogger) Errorf(format string, args ...any) {
	f.write("ERROR", format, args...)
}

func (f *fakeLogger) Warningf(format string, args ...any) {
	f.write("WARNING", format, args...)
}

func (f *fakeLogger) Infof(format string, args ...any) {
	f.write("INFO", format, args...)
}

func (f *fakeLogger) Debugf(format string, args ...any) {
	f.write("DEBUG", format, args...)
}

func (f *fakeLogger) Tracef(format string, args ...any) {
	f.write("TRACE", format, args...)
}
