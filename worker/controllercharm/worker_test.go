// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pubsub/agent"
	"github.com/juju/juju/worker/controllercharm"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (*suite) TestAddMetricsUser(c *gc.C) {
	tests := []struct {
		description string
		username    string
		initUsers   []string
		expected    string
	}{{
		description: "successfully adding metrics user",
		username:    "juju-metrics-r0",
		initUsers:   []string{},
		expected:    `successfully created user "juju-metrics-r0"`,
	}, {
		description: "user already exists",
		username:    "juju-metrics-r0",
		initUsers:   []string{"juju-metrics-r0"},
		expected:    `error creating user "juju-metrics-r0": .*`,
	}}

	for _, test := range tests {
		facade := newFakeFacade(test.initUsers...)
		hub := newFakeHub()

		_, err := controllercharm.NewWorker(controllercharm.Config{
			Facade: facade,
			Hub:    hub,
			Logger: loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)

		addMetricsUser, ok := hub.handlers[agent.AddMetricsUserTopic]
		c.Assert(ok, jc.IsTrue)
		addMetricsUser("", agent.UserInfo{
			Username: test.username,
			Password: "supersecret",
		})

		response, ok := hub.messages[agent.AddMetricsUserResponseTopic]
		c.Assert(ok, jc.IsTrue)
		c.Assert(response, gc.FitsTypeOf, "")
		c.Assert(response, gc.Matches, test.expected)
	}
}

func (*suite) TestRemoveMetricsUser(c *gc.C) {
	tests := []struct {
		description string
		username    string
		initUsers   []string
		expected    string
	}{{
		description: "successfully removing metrics user",
		username:    "juju-metrics-r0",
		initUsers:   []string{"juju-metrics-r0"},
		expected:    `successfully removed user "juju-metrics-r0"`,
	}, {
		description: "user not found",
		username:    "juju-metrics-r0",
		initUsers:   []string{},
		expected:    `error removing user "juju-metrics-r0": .*`,
	}}

	for _, test := range tests {
		facade := newFakeFacade(test.initUsers...)
		hub := newFakeHub()

		_, err := controllercharm.NewWorker(controllercharm.Config{
			Facade: facade,
			Hub:    hub,
			Logger: loggo.GetLogger("test"),
		})
		c.Assert(err, jc.ErrorIsNil)

		removeMetricsUser, ok := hub.handlers[agent.RemoveMetricsUserTopic]
		c.Assert(ok, jc.IsTrue)
		removeMetricsUser("", test.username)

		response, ok := hub.messages[agent.RemoveMetricsUserResponseTopic]
		c.Assert(ok, jc.IsTrue)
		c.Assert(response, gc.FitsTypeOf, "")
		c.Assert(response, gc.Matches, test.expected)
	}
}

type fakeFacade struct {
	users set.Strings
}

func newFakeFacade(initUsers ...string) *fakeFacade {
	return &fakeFacade{
		users: set.NewStrings(initUsers...),
	}
}

func (f *fakeFacade) AddMetricsUser(username, password string) error {
	if f.users.Contains(username) {
		return errors.AlreadyExistsf("user %q", username)
	}

	f.users.Add(username)
	return nil
}

func (f *fakeFacade) RemoveMetricsUser(username string) error {
	if !f.users.Contains(username) {
		return errors.NotFoundf("user %q", username)
	}

	f.users.Remove(username)
	return nil
}

type fakeHub struct {
	handlers map[string]func(string, any)
	messages map[string]any
}

func newFakeHub() *fakeHub {
	return &fakeHub{
		handlers: map[string]func(string, any){},
		messages: map[string]any{},
	}
}

func (h *fakeHub) Publish(topic string, data any) func() {
	h.messages[topic] = data
	return func() {}
}

func (h *fakeHub) Subscribe(topic string, handler func(string, any)) func() {
	h.handlers[topic] = handler
	return func() {
		delete(h.handlers, topic)
	}
}
