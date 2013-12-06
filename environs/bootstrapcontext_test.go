// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"os"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type bootstrapContextSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&bootstrapContextSuite{})

func (s *bootstrapContextSuite) TestNewBootstrapContext(c *gc.C) {
	ctx := environs.NewBootstrapContext()
	c.Assert(ctx, gc.NotNil)
	c.Assert(ctx.Stdin, gc.Equals, os.Stdin)
	c.Assert(ctx.Stdout, gc.Equals, os.Stdout)
	c.Assert(ctx.Stderr, gc.Equals, os.Stderr)
	oldfunc := ctx.SetInterruptHandler(nil)
	c.Assert(oldfunc, gc.IsNil)
}

func (s *bootstrapContextSuite) TestSetInterruptHandler(c *gc.C) {
	ctx := environs.NewBootstrapContext()
	proc, err := os.FindProcess(os.Getpid())
	c.Assert(err, gc.IsNil)
	defer proc.Release()

	signalled := make(chan *int, 1)
	var a, b int
	f1 := func() {
		a++
		signalled <- &a
	}
	f2 := func() {
		b++
		signalled <- &b
	}

	pairs := []struct {
		f   func()
		ptr *int
	}{
		{f1, &a},
		{f2, &b},
	}
	for i := 0; i < cap(signalled); i++ {
		for _, pair := range pairs {
			f, ptr := pair.f, pair.ptr
			ctx.SetInterruptHandler(f)
			err = proc.Signal(os.Interrupt)
			c.Assert(err, gc.IsNil)
			select {
			case received := <-signalled:
				c.Assert(received, gc.Equals, ptr)
			case <-time.After(coretesting.LongWait):
				c.Errorf("did not receive signal in time")
			}
			c.Assert(*ptr, gc.Equals, i+1)
		}
	}
}
