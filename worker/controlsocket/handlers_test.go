// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

type handlerSuite struct {
	state  *fakeState
	logger Logger
}

var _ = gc.Suite(&handlerSuite{})

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

func (s *handlerSuite) SetUpTest(c *gc.C) {
	s.state = &fakeState{}
	s.logger = loggo.GetLogger(c.TestName())
}

func (s *handlerSuite) runHandlerTest(c *gc.C, test handlerTest) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	_, err := NewWorker(Config{
		State:      s.state,
		Logger:     s.logger,
		SocketName: socket,
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

func (s *handlerSuite) assertState(c *gc.C, users []fakeUser) {
	c.Assert(len(s.state.users), gc.Equals, len(users))

	for _, expected := range users {
		actual, ok := s.state.users[expected.name]
		c.Assert(ok, gc.Equals, true)
		c.Check(actual.creator, gc.Equals, expected.creator)
		c.Check(actual.password, gc.Equals, expected.password)
	}
}

func (s *handlerSuite) TestMetricsUsersAddInvalidMethod(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *handlerSuite) TestMetricsUsersAddMissingBody(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		statusCode: http.StatusBadRequest,
		response:   ".*missing request body.*",
	})
}

func (s *handlerSuite) TestMetricsUsersAddInvalidBody(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       "username foo, password bar",
		statusCode: http.StatusBadRequest,
		response:   ".*request body is not valid JSON.*",
	})
}

func (s *handlerSuite) TestMetricsUsersAddMissingUsername(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*missing username.*",
	})
}

func (s *handlerSuite) TestMetricsUsersAddMissingPassword(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"juju-metrics-r0"}`,
		statusCode: http.StatusBadRequest,
		response:   ".*empty password.*",
	})
}

func (s *handlerSuite) TestMetricsUsersAddUsernameMissingPrefix(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodPost,
		endpoint:   "/metrics-users",
		body:       `{"username":"foo","password":"bar"}`,
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *handlerSuite) TestMetricsUsersAddSuccess(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersAddAlreadyExists(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersAddDifferentPassword(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersAddAddErr(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersAddIdempotent(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersAddFailed(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersRemoveInvalidMethod(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodGet,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusMethodNotAllowed,
		ignoreBody: true,
	})
}

func (s *handlerSuite) TestMetricsUsersRemoveUsernameMissingPrefix(c *gc.C) {
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/foo",
		statusCode: http.StatusBadRequest,
		response:   `.*username .* should have prefix \\\"juju-metrics-\\\".*`,
	})
}

func (s *handlerSuite) TestMetricsUsersRemoveSuccess(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersRemoveForbidden(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersRemoveNotFound(c *gc.C) {
	s.state = newFakeState(nil)
	s.runHandlerTest(c, handlerTest{
		method:     http.MethodDelete,
		endpoint:   "/metrics-users/juju-metrics-r0",
		statusCode: http.StatusOK, // succeed as a no-op
		response:   `.*deleted user \\\"juju-metrics-r0\\\".*`,
	})
	s.assertState(c, nil)
}

func (s *handlerSuite) TestMetricsUsersRemoveIdempotent(c *gc.C) {
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

func (s *handlerSuite) TestMetricsUsersRemoveFailed(c *gc.C) {
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
			Dial: func(_, _ string) (conn net.Conn, err error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}
