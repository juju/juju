// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmdtesting

import (
	"bytes"
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/dummy"
)

// HelpText returns a command's formatted help text.
func HelpText(command cmd.Command, name string) string {
	buff := &bytes.Buffer{}
	info := command.Info()
	info.Name = name
	f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
	command.SetFlags(f)
	buff.Write(info.Help(f))
	return buff.String()
}

type gcWriter struct {
	c      *gc.C
	source string
}

func (w *gcWriter) Write(p []byte) (n int, err error) {
	message := fmt.Sprintf("%s: %s", w.source, p)
	// Magic calldepth value...
	// The value says "how far up the call stack do we go to find the location".
	// It is used to match the standard library log function, and isn't actually
	// used by gocheck.
	w.c.Output(3, message)
	return len(p), nil
}

// NullContext returns a no-op command context.
func NullContext(c *gc.C) *cmd.Context {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = &gcWriter{c: c, source: "stdout"}
	ctx.Stderr = &gcWriter{c: c, source: "stderr"}
	return ctx
}

// RunCommandWithDummyProvider runs the command and returns channels holding the
// command's operations and errors.
func RunCommandWithDummyProvider(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	if ctx == nil {
		panic("ctx == nil")
	}
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation, 200)
	dummy.Listen(opc)
	go func() {
		defer func() {
			// signal that we're done with this ops channel.
			dummy.Listen(nil)
			// now that dummy is no longer going to send ops on
			// this channel, close it to signal to test cases
			// that we are done.
			close(opc)
		}()

		if err := InitCommand(com, args); err != nil {
			errc <- err
			return
		}

		errc <- com.Run(ctx)
	}()
	return
}
