// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type ApplicationLeaderSuite struct {
	ConnSuite
	application *state.Application
}

var _ = gc.Suite(&ApplicationLeaderSuite{})

func (s *ApplicationLeaderSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.application = s.Factory.MakeApplication(c, nil)
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *ApplicationLeaderSuite) TestReadEmpty(c *gc.C) {
	s.checkSettings(c, map[string]string{})
}

func (s *ApplicationLeaderSuite) TestWrite(c *gc.C) {
	s.writeSettings(c, map[string]string{
		"foo":     "bar",
		"baz.qux": "ping",
		"pong":    "",
		"$unset":  "foo",
	})

	s.checkSettings(c, map[string]string{
		"foo":     "bar",
		"baz.qux": "ping",
		// pong: "" value is ignored
		"$unset": "foo",
	})
}

func (s *ApplicationLeaderSuite) TestOverwrite(c *gc.C) {
	s.writeSettings(c, map[string]string{
		"one":    "foo",
		"2.0":    "bar",
		"$three": "baz",
		"fo-ur":  "qux",
	})

	s.writeSettings(c, map[string]string{
		"one":    "",
		"2.0":    "ping",
		"$three": "pong",
		"$unset": "2.0",
	})

	s.checkSettings(c, map[string]string{
		// one: "" value is cleared
		"2.0":    "ping",
		"$three": "pong",
		"fo-ur":  "qux",
		"$unset": "2.0",
	})
}

func (s *ApplicationLeaderSuite) TestTxnRevnoChange(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		s.writeSettings(c, map[string]string{
			"other":   "values",
			"slipped": "in",
			"before":  "we",
			"managed": "to",
		})
	}).Check()

	s.writeSettings(c, map[string]string{
		"but":       "we",
		"overwrite": "those",
		"before":    "",
	})

	s.checkSettings(c, map[string]string{
		"other":     "values",
		"slipped":   "in",
		"but":       "we",
		"managed":   "to",
		"overwrite": "those",
	})
}

func (s *ApplicationLeaderSuite) TestTokenError(c *gc.C) {
	err := s.application.UpdateLeaderSettings(&failToken{}, map[string]string{"blah": "blah"})
	c.Check(err, gc.ErrorMatches, `application "mysql": checking leadership continuity: something bad happened`)
}

func (s *ApplicationLeaderSuite) TestReadWriteDying(c *gc.C) {
	s.preventRemove(c)
	s.destroyApplication(c)

	s.writeSettings(c, map[string]string{
		"this":  "should",
		"still": "work",
	})
	s.checkSettings(c, map[string]string{
		"this":  "should",
		"still": "work",
	})
}

func (s *ApplicationLeaderSuite) TestReadRemoved(c *gc.C) {
	s.destroyApplication(c)

	actual, err := s.application.LeaderSettings()
	c.Check(err, gc.ErrorMatches, `application "mysql" not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(actual, gc.IsNil)
}

func (s *ApplicationLeaderSuite) TestWriteRemoved(c *gc.C) {
	s.destroyApplication(c)

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"should": "fail",
	})
	c.Check(err, gc.ErrorMatches, `application "mysql" not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *ApplicationLeaderSuite) TestWatchInitialEvent(c *gc.C) {
	w := s.application.WatchLeaderSettings()
	defer workertest.CleanKill(c, w)

	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) TestWatchDetectChange(c *gc.C) {
	w := s.application.WatchLeaderSettings()
	defer workertest.CleanKill(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) TestWatchIgnoreNullChange(c *gc.C) {
	w := s.application.WatchLeaderSettings()
	defer workertest.CleanKill(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	err = s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ApplicationLeaderSuite) TestWatchCoalesceChanges(c *gc.C) {
	w := s.application.WatchLeaderSettings()
	defer workertest.CleanKill(c, w)
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err := s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"something": "changed",
	})
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.application.UpdateLeaderSettings(&fakeToken{}, map[string]string{
		"very": "excitingly",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *ApplicationLeaderSuite) writeSettings(c *gc.C, update map[string]string) {
	err := s.application.UpdateLeaderSettings(&fakeToken{}, update)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ApplicationLeaderSuite) checkSettings(c *gc.C, expect map[string]string) {
	actual, err := s.application.LeaderSettings()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actual, gc.DeepEquals, expect)
}

func (s *ApplicationLeaderSuite) preventRemove(c *gc.C) {
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
}

func (s *ApplicationLeaderSuite) destroyApplication(c *gc.C) {
	killApplication, err := s.State.Application(s.application.Name())
	c.Assert(err, jc.ErrorIsNil)
	err = killApplication.Destroy(state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)
}

// fakeToken implements leadership.Token.
type fakeToken struct {
	err error
}

// Check is part of the leadership.Token interface. It returns its
// contained error (which defaults to nil), and never checks or writes
// the userdata.
func (t *fakeToken) Check() error {
	return t.err
}

// failToken implements leadership.Token.
type failToken struct{}

// Check is part of the leadership.Token interface. It always returns an error.
func (*failToken) Check() error {
	return errors.New("something bad happened")
}
