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
			ch <- true
		}
		v.Close()
	}()
	w := v.Watch()
	for w.Next() {
		fmt.Println(w.Value())
		<-ch
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
	got := v.Get()
	c.Assert(got, gc.Equals, expected)
	c.Assert(v.Closed(), jc.IsFalse)
}

func (s *suite) TestValueInitial(c *gc.C) {
	expected := "12345"
	v := NewValue(expected)
	got := v.Get()
	c.Assert(got, gc.Equals, expected)
	c.Assert(v.Closed(), jc.IsFalse)
}

func (s *suite) TestValueClose(c *gc.C) {
	expected := "12345"
	v := NewValue(expected)
	c.Assert(v.Close(), gc.IsNil)

	isClosed := v.Closed()
	c.Assert(isClosed, jc.IsTrue)
	got := v.Get()
	c.Assert(got, gc.Equals, expected)

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
			ch <- true
		}
		v.Close()
	}()

	w := v.Watch()
	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[0])

	// test that we can get the same value multiple times
	c.Assert(w.Value(), gc.Equals, vals[0])
	<-ch

	// now try skipping a value by calling next without getting the value
	c.Assert(w.Next(), jc.IsTrue)
	<-ch

	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[2])
	<-ch

	c.Assert(w.Next(), jc.IsFalse)
}

func (s *suite) TestDoubleSet(c *gc.C) {
	vals := []string{"one", "two", "three"}

	// blocking on the channel forces the scheduler to let the other goroutine
	// run for a bit, so we get predictable results.  This is not necessary for
	// normal use of the watcher.
	ch := make(chan bool)

	v := NewValue(nil)

	go func() {
		v.Set(vals[0])
		ch <- true
		v.Set(vals[1])
		v.Set(vals[2])
		ch <- true
		v.Close()
		ch <- true
	}()

	w := v.Watch()
	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[0])
	<-ch

	// since we did two sets before sending on the channel,
	// we should just get vals[2] here and not get vals[1]
	c.Assert(w.Next(), jc.IsTrue)
	c.Assert(w.Value(), gc.Equals, vals[2])
}

func (s *suite) TestTwoReceivers(c *gc.C) {
	vals := []string{"one", "two", "three"}

	// blocking on the channel forces the scheduler to let the other goroutine
	// run for a bit, so we get predictable results.  This is not necessary for
	// normal use of the watcher.
	ch := make(chan bool)

	v := NewValue(nil)

	watcher := func() {
		w := v.Watch()
		x := 0
		for w.Next() {
			c.Assert(w.Value(), gc.Equals, vals[x])
			x++
			<-ch
		}
		c.Assert(x, gc.Equals, len(vals))
		<-ch
	}

	go watcher()
	go watcher()

	for _, val := range vals {
		v.Set(val)
		ch <- true
		ch <- true
	}

	v.Close()
	ch <- true
	ch <- true
}

func (s *suite) TestCloseWatcher(c *gc.C) {
	vals := []string{"one", "two", "three"}

	// blocking on the channel forces the scheduler to let the other goroutine
	// run for a bit, so we get predictable results.  This is not necessary for
	// normal use of the watcher.
	ch := make(chan bool)

	v := NewValue(nil)

	w := v.Watch()
	go func() {
		x := 0
		for w.Next() {
			c.Assert(w.Value(), gc.Equals, vals[x])
			x++
			<-ch
		}
		// the value will only get set once before the watcher is closed
		c.Assert(x, gc.Equals, 1)
		<-ch
	}()

	v.Set(vals[0])
	ch <- true
	w.Close()
	ch <- true

	// prove the value is not closed, even though the watcher is
	c.Assert(v.Closed(), jc.IsFalse)
}

func (s *suite) TestWatchZeroValue(c *gc.C) {
	var v Value
	ch := make(chan bool)
	go func() {
		w := v.Watch()
		ch <- true
		ch <- w.Next()
	}()
	<-ch
	v.Set(struct{}{})
	c.Assert(<-ch, jc.IsTrue)
}
