// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	statuscmd "github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type StatusHistorySuite struct {
	testhelpers.IsolationSuite
	api statuscmd.HistoryAPI
	now time.Time
}

func TestStatusHistorySuite(t *testing.T) {
	tc.Run(t, &StatusHistorySuite{})
}

func (s *StatusHistorySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.now = time.Date(2017, 11, 28, 12, 34, 56, 0, time.UTC)
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
				Info:   "agent initialising",
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
			}, {
				Kind:   status.KindModel,
				Status: status.Suspended,
				Info:   "invalid credentials",
				Data:   map[string]interface{}{"reason": "bad password"},
				Since:  s.next(),
			},
		},
	}
}

func (s *StatusHistorySuite) newCommand() cmd.Command {
	return statuscmd.NewStatusHistoryCommandForTest(s.api)
}

func (s *StatusHistorySuite) next() *time.Time {
	value := s.now
	s.now = s.now.Add(time.Minute)
	return &value
}

func (s *StatusHistorySuite) TestMissingEntity(c *tc.C) {
	s.api = &fakeHistoryAPI{err: errors.NotFoundf("missing/0")}
	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0")
	c.Assert(err, tc.ErrorMatches, "missing/0 not found")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *StatusHistorySuite) TestTabular(c *tc.C) {
	c.Log(os.Environ())
	expected := `
Time                  Type       Status       Message
2017-11-28 12:34:56Z  juju-unit  allocating   
2017-11-28 12:35:56Z  workload   waiting      waiting for machine
2017-11-28 12:36:56Z  workload   waiting      installing agent
2017-11-28 12:37:56Z  workload   waiting      agent initialising
2017-11-28 12:38:56Z  workload   maintenance  installing charm software
2017-11-28 12:39:56Z  juju-unit  executing    running install hoook
2017-11-28 12:40:56Z  juju-unit  executing    running config-changed hoook
2017-11-28 12:41:56Z  model      suspended    invalid credentials
`[1:]
	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0", "--utc")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

func (s *StatusHistorySuite) TestYaml(c *tc.C) {
	c.Log(os.Environ())
	expected := `
- status: allocating
  since: 2017-11-28T12:34:56Z
  type: juju-unit
- status: waiting
  message: waiting for machine
  since: 2017-11-28T12:35:56Z
  type: workload
- status: waiting
  message: installing agent
  since: 2017-11-28T12:36:56Z
  type: workload
- status: waiting
  message: agent initialising
  since: 2017-11-28T12:37:56Z
  type: workload
- status: maintenance
  message: installing charm software
  since: 2017-11-28T12:38:56Z
  type: workload
- status: executing
  message: running install hoook
  since: 2017-11-28T12:39:56Z
  type: juju-unit
- status: executing
  message: running config-changed hoook
  since: 2017-11-28T12:40:56Z
  type: juju-unit
- status: suspended
  message: invalid credentials
  data:
    reason: bad password
  since: 2017-11-28T12:41:56Z
  type: model
`[1:]
	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0", "--utc", "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

// Test that multiple controllers' histories are combined, deduplicated and
// sorted deterministically.
func (s *StatusHistorySuite) TestMultiControllerCombineAndDedup(c *tc.C) {
	// Build a shared set of times to ensure deterministic output and
	// intentional duplicates.
	t0 := time.Date(2017, 11, 28, 12, 33, 56, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)

	api1 := &fakeHistoryAPI{history: status.History{
		{Kind: status.KindUnitAgent, Status: status.Pending, Since: &t0},                             // will be ignored by `-n 3` flag
		{Kind: status.KindUnitAgent, Status: status.Allocating, Since: &t1},                          // deduplicated
		{Kind: status.KindWorkload, Status: status.Waiting, Info: "waiting for machine", Since: &t2}, // deduplicated
	}}
	// api2 returns a duplicate of the two above plus an extra entry.
	api2 := &fakeHistoryAPI{history: status.History{
		{Kind: status.KindUnitAgent, Status: status.Allocating, Since: &t1},                          // deduplicated
		{Kind: status.KindWorkload, Status: status.Waiting, Info: "waiting for machine", Since: &t2}, // deduplicated
		{Kind: status.KindWorkload, Status: status.Maintenance, Info: "installing charm software", Since: &t3},
	}}

	cmd := statuscmd.NewStatusHistoryCommandForTest(api1, api2)
	ctx, err := cmdtesting.RunCommand(c, cmd, "mysql/0", "--utc", "-n", "3")
	c.Assert(err, tc.ErrorIsNil)

	expected := `
Time                  Type       Status       Message
2017-11-28 12:34:56Z  juju-unit  allocating   
2017-11-28 12:35:56Z  workload   waiting      waiting for machine
2017-11-28 12:36:56Z  workload   maintenance  installing charm software
`[1:]
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

// Test that if one controller returns an error but another returns data,
// the command prints the error to stderr and still outputs available data.
func (s *StatusHistorySuite) TestMultiControllerOneErrors(c *tc.C) {
	t0 := time.Date(2017, 11, 28, 12, 34, 56, 0, time.UTC)
	good := &fakeHistoryAPI{history: status.History{
		{Kind: status.KindUnitAgent, Status: status.Allocating, Since: &t0},
	}}
	bad := &fakeHistoryAPI{err: errors.New("controller B unavailable")}

	cmd := statuscmd.NewStatusHistoryCommandForTest(bad, good)
	ctx, err := cmdtesting.RunCommand(c, cmd, "mysql/0", "--utc")
	c.Assert(err, tc.ErrorIsNil)

	expected := `
Time                  Type       Status      Message
2017-11-28 12:34:56Z  juju-unit  allocating  
`[1:]
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	// Expect stderr to mention the error, and stdout to contain the single row.
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "controller B unavailable")
}

type fakeHistoryAPI struct {
	err     error
	history status.History
}

func (*fakeHistoryAPI) Close() error {
	return nil
}

func (f *fakeHistoryAPI) StatusHistory(ctx context.Context, kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error) {
	return f.history, f.err
}
