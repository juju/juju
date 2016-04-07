// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

// RestoreInfoSuite is *tremendously* incomplete: this test exists purely to
// verify that independent RestoreInfoSetters can be created concurrently.
// This says nothing about whether that's a good idea (it's *not*) but it's
// what we currently do and we need it to not just arbitrarily fail.
//
// TODO(fwereade): 2016-03-23 lp:1560920
// None of the other functionality is tested, and little of it is reliable or
// consistent with the other state code, but that's not for today.
type RestoreInfoSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&RestoreInfoSuite{})

func (s *RestoreInfoSuite) TestGetSetter(c *gc.C) {
	setter, err := s.State.RestoreInfoSetter()
	c.Assert(err, jc.ErrorIsNil)
	checkStatus(c, setter, state.UnknownRestoreStatus)
}

func (s *RestoreInfoSuite) TestGetSetterRace(c *gc.C) {
	trigger := make(chan struct{})
	test := func() {
		select {
		case <-trigger:
			setter, err := s.State.RestoreInfoSetter()
			if c.Check(err, jc.ErrorIsNil) {
				checkStatus(c, setter, state.UnknownRestoreStatus)
			}
		case <-time.After(coretesting.LongWait):
			c.Errorf("test invoked but not triggered")
		}
	}

	const count = 100
	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			test()
		}()
	}
	close(trigger)
	wg.Wait()
}

func checkStatus(c *gc.C, setter *state.RestoreInfo, status state.RestoreStatus) {
	if c.Check(setter, gc.NotNil) {
		c.Check(setter.Status(), gc.Equals, status)
	}
}
