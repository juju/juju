// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/client/highavailability"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
)

type StatusHistorySuite struct {
	testhelpers.IsolationSuite
	clients []HistoryAPI
	now     time.Time
}

func TestStatusHistorySuite(t *testing.T) {
	tc.Run(t, &StatusHistorySuite{})
}

func (s *StatusHistorySuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.now = time.Date(2017, 11, 28, 12, 34, 56, 0, time.UTC)
}

func (s *StatusHistorySuite) newCommand() cmd.Command {
	return NewStatusHistoryCommandForTest(s.clients)
}

func (s *StatusHistorySuite) next() *time.Time {
	value := s.now
	s.now = s.now.Add(time.Minute)
	return &value
}

func (s *StatusHistorySuite) TestMissingEntity(c *tc.C) {
	s.clients = []HistoryAPI{
		&fakeHistoryAPI{err: errors.NotFoundf("missing/0")},
	}
	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0")
	c.Assert(err, tc.ErrorMatches, "missing/0 not found")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *StatusHistorySuite) TestTabular(c *tc.C) {
	s.singularClient()

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
	s.singularClient()

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

func (s *StatusHistorySuite) TestTabularWithMultipleClients(c *tc.C) {
	s.multipleClients()

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

func (s *StatusHistorySuite) TestYamlWithMultipleClients(c *tc.C) {
	s.multipleClients()

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

func (s *StatusHistorySuite) TestClientCompatibility(c *tc.C) {
	s.clients = nil

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

	s.PatchValue(&getControllerDetailsClient, func(_ context.Context, _ *statusHistoryCommand) (ControllerDetailsAPI, error) {
		return &fakeControllerDetailsAPI{
			apiVersion: 2,
		}, nil
	})
	fake := s.singularHistoryAPI()
	s.PatchValue(&getStatusHistoryClient, func(ctx context.Context, _ *statusHistoryCommand) (HistoryAPI, error) {
		return fake, nil
	})

	ctx, err := cmdtesting.RunCommand(c, s.newCommand(), "missing/0", "--utc", "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

func (s *StatusHistorySuite) singularClient() {
	s.clients = []HistoryAPI{
		s.singularHistoryAPI(),
	}
}

func (s *StatusHistorySuite) singularHistoryAPI() HistoryAPI {
	return &fakeHistoryAPI{
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
				Data:   map[string]any{"reason": "bad password"},
				Since:  s.next(),
			},
		},
	}
}

func (s *StatusHistorySuite) multipleClients() {
	a, b, c := s.next(), s.next(), s.next()
	s.clients = []HistoryAPI{
		&fakeHistoryAPI{
			history: status.History{
				{
					Kind:   status.KindWorkload,
					Status: status.Waiting,
					Info:   "waiting for machine",
					Since:  b,
				}, {
					Kind:   status.KindUnitAgent,
					Status: status.Allocating,
					Since:  a,
				}, {
					Kind:   status.KindWorkload,
					Status: status.Waiting,
					Info:   "installing agent",
					Since:  c,
				},
			},
		},
		&fakeHistoryAPI{
			history: status.History{
				{
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
				},
			},
		},
		&fakeHistoryAPI{
			history: status.History{
				{
					Kind:   status.KindModel,
					Status: status.Suspended,
					Info:   "invalid credentials",
					Data:   map[string]any{"reason": "bad password"},
					Since:  s.next(),
				},
			},
		},
	}
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

type fakeControllerDetailsAPI struct {
	details    map[string]highavailability.ControllerDetails
	apiVersion int
}

func (api *fakeControllerDetailsAPI) ControllerDetails(ctx context.Context) (map[string]highavailability.ControllerDetails, error) {
	return api.details, nil
}

func (api *fakeControllerDetailsAPI) BestAPIVersion() int {
	return api.apiVersion
}

func (api *fakeControllerDetailsAPI) Close() error {
	return nil
}
