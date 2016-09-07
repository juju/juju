// Copyright 2016 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/authentication"
	coretesting "github.com/juju/juju/testing"
)

type InteractionsSuite struct {
	testing.IsolationSuite
	interactions *authentication.Interactions
}

var _ = gc.Suite(&InteractionsSuite{})

func (s *InteractionsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.interactions = authentication.NewInteractions()
}

func (s *InteractionsSuite) TestStart(c *gc.C) {
	waitId, err := s.interactions.Start("caveat-id", time.Time{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(waitId, gc.Not(gc.Equals), "")
}

func (s *InteractionsSuite) TestDone(c *gc.C) {
	waitId := s.start(c, "caveat-id")
	err := s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InteractionsSuite) TestDoneNotFound(c *gc.C) {
	err := s.interactions.Done("not-found", names.NewUserTag("admin@local"), nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `interaction "not-found" not found`)
}

func (s *InteractionsSuite) TestDoneTwice(c *gc.C) {
	waitId := s.start(c, "caveat-id")
	err := s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.interactions.Done(waitId, names.NewUserTag("admin@local"), nil)
	c.Assert(err, gc.ErrorMatches, `interaction ".*" already done`)
}

func (s *InteractionsSuite) TestWait(c *gc.C) {
	waitId := s.start(c, "caveat-id")
	loginUser := names.NewUserTag("admin@local")
	loginError := errors.New("login failed")
	s.done(c, waitId, loginUser, loginError)
	interaction, err := s.interactions.Wait(waitId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interaction, gc.NotNil)
	c.Assert(interaction, jc.DeepEquals, &authentication.Interaction{
		CaveatId:   "caveat-id",
		LoginUser:  loginUser,
		LoginError: loginError,
	})
}

func (s *InteractionsSuite) TestWaitNotFound(c *gc.C) {
	interaction, err := s.interactions.Wait("not-found", nil)
	c.Assert(err, gc.ErrorMatches, `interaction "not-found" not found`)
	c.Assert(interaction, gc.IsNil)
}

func (s *InteractionsSuite) TestWaitTwice(c *gc.C) {
	waitId := s.start(c, "caveat-id")
	s.done(c, waitId, names.NewUserTag("admin@local"), nil)

	_, err := s.interactions.Wait(waitId, nil)
	c.Assert(err, jc.ErrorIsNil)

	// The Wait call above should have removed the item.
	_, err = s.interactions.Wait(waitId, nil)
	c.Assert(err, gc.ErrorMatches, `interaction ".*" not found`)
}

func (s *InteractionsSuite) TestWaitCancellation(c *gc.C) {
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
		c.Assert(err, gc.Equals, authentication.ErrWaitCanceled)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Wait to return")
	}
}

func (s *InteractionsSuite) TestWaitExpired(c *gc.C) {
	t0 := time.Now()
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)

	waitId, err := s.interactions.Start("caveat-id", t2)
	c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(result.err, gc.Equals, authentication.ErrExpired)
		c.Assert(result.interaction, gc.IsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Wait to return")
	}
}

func (s *InteractionsSuite) start(c *gc.C, caveatId string) string {
	waitId, err := s.interactions.Start(caveatId, time.Time{})
	c.Assert(err, jc.ErrorIsNil)
	return waitId
}

func (s *InteractionsSuite) done(c *gc.C, waitId string, loginUser names.UserTag, loginError error) {
	err := s.interactions.Done(waitId, loginUser, loginError)
	c.Assert(err, jc.ErrorIsNil)
}
