// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type DebugLogSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&DebugLogSuite{})

func (s *DebugLogSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		args     []string
		expected common.DebugLogParams
		errMatch string
	}{
		{
			expected: common.DebugLogParams{
				Backlog: 10,
			},
		}, {
			args: []string{"-n0"},
		}, {
			args: []string{"--lines=50"},
			expected: common.DebugLogParams{
				Backlog: 50,
			},
		}, {
			args:     []string{"-l", "foo"},
			errMatch: `level value "foo" is not one of "TRACE", "DEBUG", "INFO", "WARNING", "ERROR"`,
		}, {
			args: []string{"--level=INFO"},
			expected: common.DebugLogParams{
				Backlog: 10,
				Level:   loggo.INFO,
			},
		}, {
			args: []string{
				"--include", "machine-1",
				"-i", "2",
				"--include", "unit-foo-2",
				"-i", "foo/3",
				"--include", "bar"},
			expected: common.DebugLogParams{
				IncludeEntity: []string{
					"machine-1", "machine-2",
					"unit-foo-2", "unit-foo-3",
					"unit-bar-*"},
				Backlog: 10,
			},
		}, {
			args: []string{
				"--exclude", "machine-1",
				"-x", "2",
				"--exclude", "unit-foo-2",
				"-x", "foo/3",
				"--exclude", "bar"},
			expected: common.DebugLogParams{
				ExcludeEntity: []string{
					"machine-1", "machine-2",
					"unit-foo-2", "unit-foo-3",
					"unit-bar-*"},
				Backlog: 10,
			},
		}, {
			args: []string{"--include-module", "juju.foo", "--include-module", "unit"},
			expected: common.DebugLogParams{
				IncludeModule: []string{"juju.foo", "unit"},
				Backlog:       10,
			},
		}, {
			args: []string{"--exclude-module", "juju.foo", "--exclude-module", "unit"},
			expected: common.DebugLogParams{
				ExcludeModule: []string{"juju.foo", "unit"},
				Backlog:       10,
			},
		}, {
			args: []string{"--include-labels", "logger-tags=http,apiserver"},
			expected: common.DebugLogParams{
				IncludeLabels: map[string]string{"logger-tags": "http,apiserver"},
				Backlog:       10,
			},
		}, {
			args: []string{"--exclude-labels", "logger-tags=http,apiserver"},
			expected: common.DebugLogParams{
				ExcludeLabels: map[string]string{"logger-tags": "http,apiserver"},
				Backlog:       10,
			},
		}, {
			args: []string{"--replay"},
			expected: common.DebugLogParams{
				Backlog: 10,
				Replay:  true,
			},
		}, {
			args:     []string{"--no-tail", "--tail"},
			errMatch: `setting --tail and --no-tail not valid`,
		}, {
			args:     []string{"--no-tail", "--retry"},
			errMatch: `setting --no-tail and --retry not valid`,
		}, {
			args: []string{"--limit", "100"},
			expected: common.DebugLogParams{
				Backlog: 10,
				Limit:   100,
			},
		}, {
			args:     []string{"--retry-delay", "-1s"},
			errMatch: `negative retry delay not valid`,
		},
	} {
		c.Logf("test %v", i)
		command := &debugLogCommand{}
		command.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(modelcmd.Wrap(command), test.args)
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.params, jc.DeepEquals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *DebugLogSuite) TestParamsPassed(c *gc.C) {
	fake := &fakeDebugLogAPI{}
	s.PatchValue(&getDebugLogAPI, func(_ *debugLogCommand) (DebugLogAPI, error) {
		return fake, nil
	})
	_, err := cmdtesting.RunCommand(c, newDebugLogCommand(jujuclienttesting.MinimalStore()),
		"-i", "machine-1*", "-x", "machine-1-lxd-1",
		"--include-module=juju.provisioner",
		"--lines=500",
		"--level=WARNING",
		"--no-tail",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fake.params, gc.DeepEquals, common.DebugLogParams{
		IncludeEntity: []string{"machine-1*"},
		IncludeModule: []string{"juju.provisioner"},
		ExcludeEntity: []string{"machine-1-lxd-1"},
		Backlog:       500,
		Level:         loggo.WARNING,
		NoTail:        true,
	})
}

func (s *DebugLogSuite) TestLogOutput(c *gc.C) {
	// test timezone is 6 hours east of UTC
	tz := time.FixedZone("test", 6*60*60)
	s.PatchValue(&getDebugLogAPI, func(_ *debugLogCommand) (DebugLogAPI, error) {
		return &fakeDebugLogAPI{log: []common.LogMessage{
			{
				Entity:    "machine-0",
				Timestamp: time.Date(2016, 10, 9, 8, 15, 23, 345000000, time.UTC),
				Severity:  "INFO",
				Module:    "test.module",
				Location:  "somefile.go:123",
				Message:   "this is the log output",
			},
		}}, nil
	})
	checkOutput := func(args ...string) {
		count := len(args)
		args, expected := args[:count-1], args[count-1]
		ctx, err := cmdtesting.RunCommand(c, newDebugLogCommandTZ(jujuclienttesting.MinimalStore(), tz), args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)

	}
	checkOutput(
		"machine-0: 14:15:23 INFO test.module this is the log output\n")
	checkOutput(
		"--ms",
		"machine-0: 14:15:23.345 INFO test.module this is the log output\n")
	checkOutput(
		"--utc",
		"machine-0: 08:15:23 INFO test.module this is the log output\n")
	checkOutput(
		"--date",
		"machine-0: 2016-10-09 14:15:23 INFO test.module this is the log output\n")
	checkOutput(
		"--utc", "--date",
		"machine-0: 2016-10-09 08:15:23 INFO test.module this is the log output\n")
	checkOutput(
		"--location",
		"machine-0: 14:15:23 INFO test.module somefile.go:123 this is the log output\n")
	checkOutput(
		"--format", "json",
		`{"timestamp":"2016-10-09T08:15:23.345Z","entity":"machine-0","level":"INFO","module":"test.module","location":"somefile.go:123","message":"this is the log output"}`+"\n")
}

func (s *DebugLogSuite) TestLogOutputWithLogs(c *gc.C) {
	// test timezone is 6 hours east of UTC
	tz := time.FixedZone("test", 6*60*60)
	s.PatchValue(&getDebugLogAPI, func(_ *debugLogCommand) (DebugLogAPI, error) {
		return &fakeDebugLogAPI{log: []common.LogMessage{
			{
				Entity:    "machine-0",
				Timestamp: time.Date(2016, 10, 9, 8, 15, 23, 345000000, time.UTC),
				Severity:  "INFO",
				Module:    "test.module",
				Location:  "somefile.go:123",
				Message:   "this is the log output",
				Labels:    map[string]string{"logger-tags": "http,foo"},
			},
		}}, nil
	})
	checkOutput := func(args ...string) {
		count := len(args)
		args, expected := args[:count-1], args[count-1]
		ctx, err := cmdtesting.RunCommand(c, newDebugLogCommandTZ(jujuclienttesting.MinimalStore(), tz), args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)

	}
	checkOutput(
		"machine-0: 14:15:23 INFO test.module logger-tags:http,foo this is the log output\n")
	checkOutput(
		"--ms",
		"machine-0: 14:15:23.345 INFO test.module logger-tags:http,foo this is the log output\n")
	checkOutput(
		"--utc",
		"machine-0: 08:15:23 INFO test.module logger-tags:http,foo this is the log output\n")
	checkOutput(
		"--date",
		"machine-0: 2016-10-09 14:15:23 INFO test.module logger-tags:http,foo this is the log output\n")
	checkOutput(
		"--utc", "--date",
		"machine-0: 2016-10-09 08:15:23 INFO test.module logger-tags:http,foo this is the log output\n")
	checkOutput(
		"--location",
		"machine-0: 14:15:23 INFO test.module somefile.go:123 logger-tags:http,foo this is the log output\n")
}

type fakeDebugLogAPI struct {
	log    []common.LogMessage
	params common.DebugLogParams
	err    error
}

func (fake *fakeDebugLogAPI) WatchDebugLog(params common.DebugLogParams) (<-chan common.LogMessage, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	fake.params = params
	response := make(chan common.LogMessage)
	go func() {
		defer close(response)
		for _, msg := range fake.log {
			response <- msg
		}
	}()
	return response, nil
}

func (fake *fakeDebugLogAPI) Close() error {
	return nil
}
