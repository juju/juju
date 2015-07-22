// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package looputil_test

import gc "gopkg.in/check.v1"

type mockRunCommand struct {
	c        *gc.C
	commands []*mockCommand
}

type mockCommand struct {
	cmd    string
	args   []string
	result string
	err    error
}

func (m *mockCommand) respond(result string, err error) {
	m.result = result
	m.err = err
}

func (m *mockRunCommand) expect(cmd string, args ...string) *mockCommand {
	command := &mockCommand{cmd: cmd, args: args}
	m.commands = append(m.commands, command)
	return command
}

func (m *mockRunCommand) assertDrained() {
	m.c.Assert(m.commands, gc.HasLen, 0)
}

func (m *mockRunCommand) run(cmd string, args ...string) (stdout string, err error) {
	m.c.Assert(m.commands, gc.Not(gc.HasLen), 0)
	expect := m.commands[0]
	m.commands = m.commands[1:]
	m.c.Assert(cmd, gc.Equals, expect.cmd)
	m.c.Assert(args, gc.DeepEquals, expect.args)
	return expect.result, expect.err
}
