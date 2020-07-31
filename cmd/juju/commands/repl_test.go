// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&ReplSuite{})

type ReplSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (*ReplSuite) TestPromptNoControllersAtAll(c *gc.C) {
	r := &replCommand{store: jujuclient.NewMemStore()}

	_, err := r.getPrompt()
	c.Assert(err, gc.ErrorMatches, `No controllers registered.

Please either create a new controller using "juju bootstrap" or connect to
another controller that you have been given access to using "juju register".
`)
}

func (*ReplSuite) TestPromptNoCurrentController(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "$ ")
}

func (*ReplSuite) TestPromptControllerNoLogin(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "somecontroller$ ")
}

func (*ReplSuite) TestPromptControllerNoModel(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"
	store.Accounts["somecontroller"] = jujuclient.AccountDetails{User: "fred"}
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "fred@somecontroller$ ")
}

func (*ReplSuite) TestPromptControllerWithModel(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"
	store.Accounts["somecontroller"] = jujuclient.AccountDetails{User: "fred"}
	store.Models["somecontroller"] = &jujuclient.ControllerModels{
		CurrentModel: "fred/test",
	}
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "fred@somecontroller:test$ ")
}

func (*ReplSuite) TestPromptControllerWithModelDifferentUser(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"
	store.Accounts["somecontroller"] = jujuclient.AccountDetails{User: "fred"}
	store.Models["somecontroller"] = &jujuclient.ControllerModels{
		CurrentModel: "mary/test",
	}
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "fred@somecontroller:mary/test$ ")
}

func (*ReplSuite) TestPromptControllerWithModelNoLogin(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"
	store.Models["somecontroller"] = &jujuclient.ControllerModels{
		CurrentModel: "test",
	}
	r := &replCommand{store: store}

	p, err := r.getPrompt()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, "somecontroller:test$ ")
}

func (s *ReplSuite) assertOutMatches(c *gc.C, out []byte, match string) {
	stripped := strings.Replace(string(out), "\n", "", -1)
	c.Assert(stripped, gc.Matches, match)
}

func (s *ReplSuite) TestHelp(c *gc.C) {
	for _, arg := range []string{"-h", "--help", "help"} {
		s.assertHelp(c, arg)
	}
}

func (s *ReplSuite) assertHelp(c *gc.C, helpArg string) {
	f := func() {
		main{
			execCommand: func(command string, args ...string) *exec.Cmd {
				c.Fail()
				return nil
			},
		}.Run([]string{"juju", helpArg})
	}

	stdout, _ := jujutesting.CaptureOutput(c, f)
	s.assertOutMatches(c, stdout,
		"When run without arguments, enter an interactive shell.*")
}

func (s *ReplSuite) TestJujuCommandHelp(c *gc.C) {
	f := func() {
		main{
			execCommand: func(command string, args ...string) *exec.Cmd {
				c.Fail()
				return nil
			},
		}.Run([]string{"juju", "help", "status"})
	}

	stdout, _ := jujutesting.CaptureOutput(c, f)
	s.assertOutMatches(c, stdout,
		"Usage: juju show-status.*")
}

func (s *ReplSuite) TestRepl(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["somecontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "somecontroller"

	var cmds []string
	r := &replCommand{
		store: store,
		execJujuCommand: func(c cmd.Command, _ *cmd.Context, args []string) int {
			cmds = append(cmds, c.Info().Name+" "+strings.Join(args, " "))
			return 0
		},
	}
	err := cmdtesting.InitCommand(r, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: bytes.NewBuffer(nil),
		Stderr: bytes.NewBuffer(nil),
		Stdin:  bytes.NewReader([]byte("\nstatus --format yaml\ncontrollers\n\nQ\n")),
	}
	err = r.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmds, gc.HasLen, 2)
	c.Assert(cmds, jc.DeepEquals, []string{
		"juju status --format yaml",
		"juju controllers",
	})
}
