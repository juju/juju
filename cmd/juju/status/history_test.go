// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	statuscmd "github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/core/status"
)

type StatusHistorySuite struct {
	testing.IsolationSuite
	api statuscmd.HistoryAPI
	now time.Time
}

var _ = gc.Suite(&StatusHistorySuite{})

func (s *StatusHistorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.api = nil
	s.now = time.Date(2017, 11, 28, 12, 34, 56, 0, time.UTC)
}

func (s *StatusHistorySuite) newCommand() cmd.Command {
	return statuscmd.NewTestStatusHistoryCommand(s.api)
}

func (s *StatusHistorySuite) next() *time.Time {
	value := s.now
	s.now = s.now.Add(time.Minute)
	return &value
}

func (s *StatusHistorySuite) TestMissingEntity(c *gc.C) {
	s.api = &fakeHistoryAPI{err: errors.NotFoundf("missing/0")}
	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0")
	c.Assert(err, gc.ErrorMatches, "missing/0 not found")
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *StatusHistorySuite) TestResults(c *gc.C) {
	c.Log(os.Environ())

	s.api = &fakeHistoryAPI{
		history: status.History{
			{
				Kind:   status.KindUnitAgent,
				Status: status.Allocating,
				Since:  s.next(),
			}, {
				Kind:   status.KindWorkload,
				Status: status.Waiting,
				Info:   "waiting for machine",
				Since:  s.next(),
			}, {
				Kind:   status.KindWorkload,
				Status: status.Waiting,
				Info:   "installing agent",
				Since:  s.next(),
			}, {
				Kind:   status.KindWorkload,
				Status: status.Waiting,
				Info:   "agent initializing",
				Since:  s.next(),
			}, {
				Kind:   status.KindWorkload,
				Status: status.Maintenance,
				Info:   "installing charm software",
				Since:  s.next(),
			}, {
				Kind:   status.KindUnitAgent,
				Status: status.Executing,
				Info:   "running install hoook",
				Since:  s.next(),
			}, {
				Kind:   status.KindUnitAgent,
				Status: status.Executing,
				Info:   "running config-changed hoook",
				Since:  s.next(),
			},
		},
	}
	expected := "" +
		"Time                  Type       Status       Message\n" +
		"2017-11-28 12:34:56Z  juju-unit  allocating   \n" +
		"2017-11-28 12:35:56Z  workload   waiting      waiting for machine\n" +
		"2017-11-28 12:36:56Z  workload   waiting      installing agent\n" +
		"2017-11-28 12:37:56Z  workload   waiting      agent initializing\n" +
		"2017-11-28 12:38:56Z  workload   maintenance  installing charm software\n" +
		"2017-11-28 12:39:56Z  juju-unit  executing    running install hoook\n" +
		"2017-11-28 12:40:56Z  juju-unit  executing    running config-changed hoook\n"

	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0", "--utc")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)
}

type fakeHistoryAPI struct {
	err     error
	history status.History
}

func (*fakeHistoryAPI) Close() error {
	return nil
}

func (f *fakeHistoryAPI) StatusHistory(kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error) {
	return f.history, f.err
}
