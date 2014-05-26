// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
)

type DebugLogSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&DebugLogSuite{})

func (s *DebugLogSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		args     []string
		expected api.DebugLogParams
		errMatch string
	}{
		{
			expected: api.DebugLogParams{
				Backlog: 10,
			},
		}, {
			args: []string{"-n0"},
		}, {
			args: []string{"--lines=50"},
			expected: api.DebugLogParams{
				Backlog: 50,
			},
		}, {
			args:     []string{"-l", "foo"},
			errMatch: `level value "foo" is not one of "TRACE", "DEBUG", "INFO", "WARNING", "ERROR"`,
		}, {
			args: []string{"--level=INFO"},
			expected: api.DebugLogParams{
				Backlog: 10,
				Level:   loggo.INFO,
			},
		}, {
			args: []string{"--include", "machine-1", "-i", "machine-2"},
			expected: api.DebugLogParams{
				IncludeEntity: []string{"machine-1", "machine-2"},
				Backlog:       10,
			},
		}, {
			args: []string{"--exclude", "machine-1", "-x", "machine-2"},
			expected: api.DebugLogParams{
				ExcludeEntity: []string{"machine-1", "machine-2"},
				Backlog:       10,
			},
		}, {
			args: []string{"--include-module", "juju.foo", "--include-module", "unit"},
			expected: api.DebugLogParams{
				IncludeModule: []string{"juju.foo", "unit"},
				Backlog:       10,
			},
		}, {
			args: []string{"--exclude-module", "juju.foo", "--exclude-module", "unit"},
			expected: api.DebugLogParams{
				ExcludeModule: []string{"juju.foo", "unit"},
				Backlog:       10,
			},
		}, {
			args: []string{"--replay"},
			expected: api.DebugLogParams{
				Backlog: 10,
				Replay:  true,
			},
		}, {
			args: []string{"--limit", "100"},
			expected: api.DebugLogParams{
				Backlog: 10,
				Limit:   100,
			},
		},
	} {
		c.Logf("test %v", i)
		command := &DebugLogCommand{}
		err := testing.InitCommand(envcmd.Wrap(command), test.args)
		if test.errMatch == "" {
			c.Check(err, gc.IsNil)
			c.Check(command.params, jc.DeepEquals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *DebugLogSuite) TestParamsPassed(c *gc.C) {
	fake := &fakeDebugLogAPI{}
	s.PatchValue(&getDebugLogAPI, func(envName string) (DebugLogAPI, error) {
		return fake, nil
	})
	_, err := testing.RunCommand(c, envcmd.Wrap(&DebugLogCommand{}),
		"-i", "machine-1*", "-x", "machine-1-lxc-1",
		"--include-module=juju.provisioner",
		"--lines=500",
		"--level=WARNING",
	)
	c.Assert(err, gc.IsNil)
	c.Assert(fake.params, gc.DeepEquals, api.DebugLogParams{
		IncludeEntity: []string{"machine-1*"},
		IncludeModule: []string{"juju.provisioner"},
		ExcludeEntity: []string{"machine-1-lxc-1"},
		Backlog:       500,
		Level:         loggo.WARNING,
	})
}

func (s *DebugLogSuite) TestLogOutput(c *gc.C) {
	s.PatchValue(&getDebugLogAPI, func(envName string) (DebugLogAPI, error) {
		return &fakeDebugLogAPI{log: "this is the log output"}, nil
	})
	ctx, err := testing.RunCommand(c, envcmd.Wrap(&DebugLogCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "this is the log output")
}

func (s *DebugLogSuite) TestTailFallback(c *gc.C) {
	s.PatchValue(&runSSHCommand, func(sshCmd *SSHCommand, ctx *cmd.Context) error {
		fmt.Fprintf(ctx.Stdout, "%s", sshCmd.Args)
		return nil
	})
	s.PatchValue(&getDebugLogAPI, func(envName string) (DebugLogAPI, error) {
		return &fakeDebugLogAPI{err: errors.NotSupportedf("testing")}, nil
	})
	ctx, err := testing.RunCommand(c, envcmd.Wrap(&DebugLogCommand{}), "-n", "100")
	c.Assert(err, gc.IsNil)
	c.Check(testing.Stderr(ctx), gc.Equals, "Server does not support new stream log, falling back to tail\n")
	c.Check(testing.Stdout(ctx), gc.Equals, "[tail -n -100 -f /var/log/juju/all-machines.log]")
}

func newFakeDebugLogAPI(log string) DebugLogAPI {
	return &fakeDebugLogAPI{log: log}
}

type fakeDebugLogAPI struct {
	log    string
	params api.DebugLogParams
	err    error
}

func (fake *fakeDebugLogAPI) WatchDebugLog(params api.DebugLogParams) (io.ReadCloser, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	fake.params = params
	return ioutil.NopCloser(strings.NewReader(fake.log)), nil
}

func (fake *fakeDebugLogAPI) Close() error {
	return nil
}
