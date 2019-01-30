// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	//	"container/list"
	//	"fmt"
	//	"sync"
	//	"time"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"

	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&watcherSuite{})

type watcherSuite struct {
	testing.StateSuite
}

func (s *watcherSuite) TestEntityWatcherEvents(c *gc.C) {
	// Just watching a document should not trigger an event
	w := state.NewEntityWatcher(s.State, "coll", "id")
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()
}
