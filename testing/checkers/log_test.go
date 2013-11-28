// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	jc "launchpad.net/juju-core/testing/checkers"
)

type LogMatchesSuite struct{}

var _ = gc.Suite(&LogMatchesSuite{})

func (s *LogMatchesSuite) TestMatchSimpleMessage(c *gc.C) {
	log := []loggo.TestLogValues{
		{Level: loggo.INFO, Message: "foo bar"},
		{Level: loggo.INFO, Message: "12345"},
	}
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "foo bar"},
		{loggo.INFO, "12345"},
	})
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.INFO, "foo .*"},
		{loggo.INFO, "12345"},
	})
	// UNSPECIFIED means we don't care what the level is,
	// just check the message string matches.
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.UNSPECIFIED, "foo .*"},
		{loggo.INFO, "12345"},
	})
	c.Check(log, gc.Not(jc.LogMatches), []jc.SimpleMessage{
		{loggo.INFO, "foo bar"},
		{loggo.DEBUG, "12345"},
	})
}

func (s *LogMatchesSuite) TestMatchStrings(c *gc.C) {
	log := []loggo.TestLogValues{
		{Level: loggo.INFO, Message: "foo bar"},
		{Level: loggo.INFO, Message: "12345"},
	}
	c.Check(log, jc.LogMatches, []string{"foo bar", "12345"})
	c.Check(log, jc.LogMatches, []string{"foo .*", "12345"})
	c.Check(log, gc.Not(jc.LogMatches), []string{"baz", "bing"})
}

func (s *LogMatchesSuite) TestMatchInexact(c *gc.C) {
	log := []loggo.TestLogValues{
		{Level: loggo.INFO, Message: "foo bar"},
		{Level: loggo.INFO, Message: "baz"},
		{Level: loggo.DEBUG, Message: "12345"},
		{Level: loggo.ERROR, Message: "12345"},
		{Level: loggo.INFO, Message: "67890"},
	}
	c.Check(log, jc.LogMatches, []string{"foo bar", "12345"})
	c.Check(log, jc.LogMatches, []string{"foo .*", "12345"})
	c.Check(log, jc.LogMatches, []string{"foo .*", "67890"})
	c.Check(log, jc.LogMatches, []string{"67890"})

	// Matches are always left-most after the previous match.
	c.Check(log, jc.LogMatches, []string{".*", "baz"})
	c.Check(log, jc.LogMatches, []string{"foo bar", ".*", "12345"})
	c.Check(log, jc.LogMatches, []string{"foo bar", ".*", "67890"})

	// Order is important: 67890 advances to the last item in obtained,
	// and so there's nothing after to match against ".*".
	c.Check(log, gc.Not(jc.LogMatches), []string{"67890", ".*"})
	// ALL specified patterns MUST match in the order given.
	c.Check(log, gc.Not(jc.LogMatches), []string{".*", "foo bar"})

	// Check that levels are matched.
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.UNSPECIFIED, "12345"},
		{loggo.UNSPECIFIED, "12345"},
	})
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.DEBUG, "12345"},
		{loggo.ERROR, "12345"},
	})
	c.Check(log, jc.LogMatches, []jc.SimpleMessage{
		{loggo.DEBUG, "12345"},
		{loggo.INFO, ".*"},
	})
	c.Check(log, gc.Not(jc.LogMatches), []jc.SimpleMessage{
		{loggo.DEBUG, "12345"},
		{loggo.INFO, ".*"},
		{loggo.UNSPECIFIED, ".*"},
	})
}

func (s *LogMatchesSuite) TestFromLogMatches(c *gc.C) {
	tw := &loggo.TestWriter{}
	_, err := loggo.ReplaceDefaultWriter(tw)
	c.Assert(err, gc.IsNil)
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	logger.Infof("foo")
	logger.Debugf("bar")
	logger.Tracef("hidden")
	c.Check(tw.Log, jc.LogMatches, []string{"foo", "bar"})
	c.Check(tw.Log, gc.Not(jc.LogMatches), []string{"foo", "bad"})
	c.Check(tw.Log, gc.Not(jc.LogMatches), []jc.SimpleMessage{
		{loggo.INFO, "foo"},
		{loggo.INFO, "bar"},
	})
}
