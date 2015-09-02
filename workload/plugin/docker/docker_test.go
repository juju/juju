// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju-process-docker/docker"
)

var _ = gc.Suite(&dockerSuite{})

type dockerSuite struct {
	testing.CleanupSuite
}

func newClient(out string) (*docker.CLIClient, *fakeRunDocker) {
	fake := &fakeRunDocker{
		calls: []runDockerCall{{
			out: []byte(out),
		}},
	}
	client := docker.NewCLIClient()
	client.RunDocker = fake.exec
	return client, fake
}

func (dockerSuite) TestRunOkay(c *gc.C) {
	client, fake := newClient("eggs")

	args := docker.RunArgs{
		Name:    "spam",
		Image:   "my-spam",
		Command: "do something",
		EnvVars: map[string]string{
			"FOO": "bar",
		},
	}
	id, err := client.Run(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, "eggs")
	c.Check(fake.index, gc.Equals, 1)
	c.Check(fake.calls[0].commandIn, gc.Equals, "run")
	c.Check(fake.calls[0].argsIn, jc.DeepEquals, []string{
		"--detach",
		"--name", "spam",
		"-e", "FOO=bar",
		"my-spam",
		"do", "something",
	})
}

func (dockerSuite) TestRunMinimal(c *gc.C) {
	client, fake := newClient("eggs")

	args := docker.RunArgs{
		Image: "my-spam",
	}
	id, err := client.Run(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, "eggs")
	c.Check(fake.index, gc.Equals, 1)
	c.Check(fake.calls[0].commandIn, gc.Equals, "run")
	c.Check(fake.calls[0].argsIn, jc.DeepEquals, []string{
		"--detach",
		"my-spam",
	})
}

func (dockerSuite) TestInspectOkay(c *gc.C) {
	client, fake := newClient(fakeInspectOutput)

	info, err := client.Inspect("sad_perlman")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, (*docker.Info)(fakeInfo))
	c.Check(fake.index, gc.Equals, 1)
	c.Check(fake.calls[0].commandIn, gc.Equals, "inspect")
	c.Check(fake.calls[0].argsIn, jc.DeepEquals, []string{
		"sad_perlman",
	})
}

func (dockerSuite) TestStopOkay(c *gc.C) {
	client, fake := newClient("")

	err := client.Stop("sad_perlman")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fake.index, gc.Equals, 1)
	c.Check(fake.calls[0].commandIn, gc.Equals, "stop")
	c.Check(fake.calls[0].argsIn, jc.DeepEquals, []string{
		"sad_perlman",
	})
}

func (dockerSuite) TestRemoveOkay(c *gc.C) {
	client, fake := newClient("")

	err := client.Remove("sad_perlman")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fake.index, gc.Equals, 1)
	c.Check(fake.calls[0].commandIn, gc.Equals, "rm")
	c.Check(fake.calls[0].argsIn, jc.DeepEquals, []string{
		"sad_perlman",
	})
}

type runDockerCall struct {
	out      []byte
	err      string
	exitcode int

	commandIn string
	argsIn    []string
}

type fakeRunDocker struct {
	calls []runDockerCall
	index int
}

// checkArgs verifies the args being passed to docker.
func (fakeRunDocker) checkArgs(command string, args []string) error {
	if len(args) < 1 {
		fullArgs := append([]string{command}, args...)
		return fmt.Errorf("Not enough arguments passed to docker: %#v\n", fullArgs)
	}
	return nil
}

func (frd *fakeRunDocker) exec(command string, args ...string) (_ []byte, rErr error) {
	frd.calls[frd.index].commandIn = command
	frd.calls[frd.index].argsIn = args
	call := frd.calls[frd.index]
	frd.index += 1

	exitcode := call.exitcode
	defer func() {
		if rErr == nil && exitcode != 0 {
			rErr = fmt.Errorf("ERROR")
		}
		if rErr != nil {
			if exitcode == 0 {
				exitcode = 1
			}
			rErr = fmt.Errorf("exit status %d: %v", exitcode, rErr)
		}
	}()

	if err := frd.checkArgs(command, args); err != nil {
		exitcode = 2
		return nil, err
	}

	if call.err != "" {
		return nil, fmt.Errorf(call.err)
	}
	return call.out, rErr
}
