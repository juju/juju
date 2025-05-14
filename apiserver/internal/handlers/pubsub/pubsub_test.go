// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	jujuhttp "github.com/juju/juju/internal/http"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type pubsubSuite struct {
	jujutesting.ApiServerSuite
	machineTag names.Tag
	password   string
	nonce      string
	hub        *pubsub.StructuredHub
	pubsubURL  string
}

var _ = tc.Suite(&pubsubSuite{})

func (s *pubsubSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.nonce = "nonce"
	m, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: s.nonce,
		Jobs:  []state.MachineJob{state.JobManageModel},
	})
	s.machineTag = m.Tag()
	s.password = password
	s.hub = s.Server.GetCentralHub()

	pubsubURL := s.URL(fmt.Sprintf("/model/%s/pubsub", s.ControllerModelUUID()), url.Values{})
	pubsubURL.Scheme = "wss"
	s.pubsubURL = pubsubURL.String()
}

func (s *pubsubSuite) TestNoAuth(c *tc.C) {
	s.checkAuthFails(c, nil, http.StatusUnauthorized, "authentication failed: no credentials provided")
}

func (s *pubsubSuite) TestRejectsUserLogins(c *tc.C) {
	userService := s.ControllerDomainServices(c).Access()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := userService.AddUser(c.Context(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	header := jujuhttp.BasicAuthHeader(userTag.String(), "hunter2")
	s.checkAuthFails(c, header, http.StatusForbidden, "authorization failed: user .* is not a controller")
}

func (s *pubsubSuite) TestRejectsNonServerMachineLogins(c *tc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	m, password := f.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "a-nonce",
		Jobs:  []state.MachineJob{state.JobHostUnits},
	})
	header := jujuhttp.BasicAuthHeader(m.Tag().String(), password)
	header.Add(params.MachineNonceHeader, "a-nonce")
	s.checkAuthFails(c, header, http.StatusForbidden, "authorization failed: machine .* is not a controller")
}

func (s *pubsubSuite) TestRejectsBadPassword(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.machineTag.String(), "wrong")
	header.Add(params.MachineNonceHeader, s.nonce)
	s.checkAuthFails(c, header, http.StatusUnauthorized, "authentication failed: invalid entity name or password")
}

func (s *pubsubSuite) TestRejectsIncorrectNonce(c *tc.C) {
	header := jujuhttp.BasicAuthHeader(s.machineTag.String(), s.password)
	header.Add(params.MachineNonceHeader, "wrong")
	s.checkAuthFails(c, header, http.StatusUnauthorized, "authentication failed: machine 0 not provisioned")
}

func (s *pubsubSuite) checkAuthFails(c *tc.C, header http.Header, code int, message string) {
	conn, resp, err := s.dialWebsocketInternal(c, header)
	c.Assert(err, tc.Equals, websocket.ErrBadHandshake)
	c.Assert(conn, tc.IsNil)
	c.Assert(resp, tc.NotNil)
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, code)
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Matches, message+"\n")
}

func (s *pubsubSuite) TestMessage(c *tc.C) {
	messages := []params.PubSubMessage{}
	done := make(chan struct{})
	loggo.GetLogger("pubsub").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	_, err := s.hub.SubscribeMatch(pubsub.MatchAll, func(topic string, data map[string]interface{}) {
		c.Logf("topic: %q, data: %v", topic, data)
		messages = append(messages, params.PubSubMessage{
			Topic: topic,
			Data:  data,
		})
		done <- struct{}{}
	})
	c.Assert(err, tc.ErrorIsNil)

	conn := s.dialWebsocket(c)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	message1 := params.PubSubMessage{
		Topic: "first",
		Data: map[string]interface{}{
			"origin":  "other",
			"message": "first message",
		}}
	err = conn.WriteJSON(&message1)
	c.Assert(err, tc.ErrorIsNil)

	message2 := params.PubSubMessage{
		Topic: "second",
		Data: map[string]interface{}{
			"origin": "other",
			"value":  false,
		}}
	err = conn.WriteJSON(&message2)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no second message")
	}

	// Close connection.
	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(messages, tc.DeepEquals, []params.PubSubMessage{message1, message2})
}

func (s *pubsubSuite) dialWebsocket(c *tc.C) *websocket.Conn {
	conn, _, err := s.dialWebsocketInternal(c, s.makeAuthHeader())
	c.Assert(err, tc.ErrorIsNil)
	return conn
}

func (s *pubsubSuite) dialWebsocketInternal(c *tc.C, header http.Header) (*websocket.Conn, *http.Response, error) {
	return dialWebsocketFromURL(c, s.pubsubURL, header)
}

func (s *pubsubSuite) makeAuthHeader() http.Header {
	header := jujuhttp.BasicAuthHeader(s.machineTag.String(), s.password)
	header.Add(params.MachineNonceHeader, s.nonce)
	return header
}
