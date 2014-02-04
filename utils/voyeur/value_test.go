// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package voyeur

import (
	"fmt"
	"testing"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type suite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&suite{})

func Test(t *testing.T) { gc.TestingT(t) }

func ExampleWatcher_Next() {
	v := NewValue(nil)

	// The channel is not necessary for normal use of the watcher.
	// It just makes the test output predictable.
	ch := make(chan bool)

	go func() {
		for x := 0; x < 3; x++ {
			v.Set(fmt.Sprintf("value%d", x))
			<-ch
		}
		v.Close()
	}()
	w := v.Watch()
	for w.Next() {
		fmt.Println(w.Value())
		ch <- true
	}

	// output:
	// value0
	// value1
	// value2
}

func (s *suite) TestValueGetSet(c *gc.C) {
	v := NewValue(nil)
	expected := "12345"
	v.Set(expected)
	got, ok := v.Get()
	c.Assert(ok, jc.IsTrue)
	c.Assert(got, gc.Equals, expected)
}

func (s *suite) TestValueInitial(c *gc.C) {
	expected := "12345"
	v := NewValue(expected)
	got, ok := v.Get()
	c.Assert(ok, jc.IsTrue)
	c.Assert(got, gc.Equals, expected)
}

func (s *suite) TestValueClose(c *gc.C) {
	v := NewValue("12345")
	c.Assert(v.Close(), gc.IsNil)
	got, ok := v.Get()
	c.Assert(ok, jc.IsFalse)
	c.Assert(got, gc.IsNil)

	// test that we can close multiple times without a problem
	c.Assert(v.Close(), gc.IsNil)
}

func (s *suite) TestWatcher(c *gc.C) {
	vals := []string{"one", "two", "three"}

	// blocking on the channel forces the scheduler to let the other goroutine
	// run for a bit, so we get predictable results.  This is not necessary for
	// normal use of the watcher.
	ch := make(chan bool)

	v := NewValue(nil)

	go func() {
		for _, s := range vals {
			v.Set(s)
			<-ch
		}
		v.Close()
	}()

	w := v.Watch()
	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[0])

	// test that we can get the same value multiple times
	c.Assert(w.Value(), gc.Equals, vals[0])
	ch <- true

	// now try skipping a value by calling next without getting the value
	c.Assert(w.Next(), jc.IsTrue)
	ch <- true

	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[2])
	ch <- true

	c.Assert(w.Next(), jc.IsFalse)
}
