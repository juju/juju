// Copyright 2016 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type InteractionsSuite struct {
	testhelpers.IsolationSuite
	interactions *authentication.Interactions
}

func TestInteractionsSuite(t *stdtesting.T) { tc.Run(t, &InteractionsSuite{}) }
func (s *InteractionsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.interactions = authentication.NewInteractions()
}

func (s *InteractionsSuite) TestStart(c *tc.C) {
	waitId, err := s.interactions.Start([]byte("caveat-id"), time.Time{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(waitId, tc.Not(tc.Equals), "")
}

func (s *InteractionsSuite) TestDone(c *tc.C) {
	waitId := s.start(c, "caveat-id")
	err := s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *InteractionsSuite) TestDoneNotFound(c *tc.C) {
	err := s.interactions.Done("not-found", names.NewUserTag("admin@local"), nil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(err, tc.ErrorMatches, `interaction "not-found" not found`)
}

func (s *InteractionsSuite) TestDoneTwice(c *tc.C) {
	waitId := s.start(c, "caveat-id")
	err := s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, tc.ErrorMatches, `interaction ".*" already done`)
}

func (s *InteractionsSuite) TestWait(c *tc.C) {
	waitId := s.start(c, "caveat-id")
	loginUser := names.NewUserTag("admin@local")
	loginError := errors.New("login failed")
	s.done(c, waitId, loginUser, loginError)
	interaction, err := s.interactions.Wait(waitId, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(interaction, tc.NotNil)
	c.Assert(interaction, tc.DeepEquals, &authentication.Interaction{
		CaveatId:   []byte("caveat-id"),
		LoginUser:  loginUser,
		LoginError: loginError,
	})
}

func (s *InteractionsSuite) TestWaitNotFound(c *tc.C) {
	interaction, err := s.interactions.Wait("not-found", nil)
	c.Assert(err, tc.ErrorMatches, `interaction "not-found" not found`)
	c.Assert(interaction, tc.IsNil)
}

func (s *InteractionsSuite) TestWaitTwice(c *tc.C) {
	waitId := s.start(c, "caveat-id")
	s.done(c, waitId, names.NewUserTag("admin@local"), nil)

	_, err := s.interactions.Wait(waitId, nil)
	c.Assert(err, tc.ErrorIsNil)

	// The Wait call above should have removed the item.
	_, err = s.interactions.Wait(waitId, nil)
	c.Assert(err, tc.ErrorMatches, `interaction ".*" not found`)
}

func (s *InteractionsSuite) TestWaitCancellation(c *tc.C) {
	waitId := s.start(c, "caveat-id")

	cancel := make(chan struct{})
	waitResult := make(chan error)
	go func() {
		_, err := s.interactions.Wait(waitId, cancel)
		waitResult <- err
	}()

	// Wait should not pass until we've cancelled.
	select {
	case err := <-waitResult:
		c.Fatalf("unexpected result: %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	cancel <- struct{}{}
	select {
	case err := <-waitResult:
		c.Assert(err, tc.Equals, authentication.ErrWaitCanceled)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Wait to return")
	}
}

func (s *InteractionsSuite) TestWaitExpired(c *tc.C) {
	t0 := time.Now()
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)

	waitId, err := s.interactions.Start([]byte("caveat-id"), t2)
	c.Assert(err, tc.ErrorIsNil)

	type waitResult struct {
		interaction *authentication.Interaction
		err         error
	}
	waitResultC := make(chan waitResult)
	go func() {
		interaction, err := s.interactions.Wait(waitId, nil)
		waitResultC <- waitResult{interaction, err}
	}()

	// This should do nothing, because there's nothing
	// due to expire until t2.
	s.interactions.Expire(t1)

	// Wait should not pass until the interaction expires.
	select {
	case result := <-waitResultC:
		c.Fatalf("unexpected result: %v", result)
	case <-time.After(coretesting.ShortWait):
	}

	s.interactions.Expire(t2)
	select {
	case result := <-waitResultC:
		c.Assert(result.err, tc.Equals, authentication.ErrExpired)
		c.Assert(result.interaction, tc.IsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Wait to return")
	}
}

func (s *InteractionsSuite) start(c *tc.C, caveatId string) string {
	waitId, err := s.interactions.Start([]byte(caveatId), time.Time{})
	c.Assert(err, tc.ErrorIsNil)
	return waitId
}

func (s *InteractionsSuite) done(c *tc.C, waitId string, loginUser names.UserTag, loginError error) {
	err := s.interactions.Done(waitId, loginUser, loginError)
	c.Assert(err, tc.ErrorIsNil)
}
