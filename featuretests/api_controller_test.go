// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
)

// This suite only exists because no user facing calls exercise
// the WatchModelSummaries or WatchAllModelSummaries.
// The primary caller is the JAAS dashboard which uses the javascript
// library. It is expected that JIMM will call these methods using
// the Go API.

type ControllerSuite struct {
	testing.JujuConnSuite
	client *controller.Client
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	userConn := s.OpenControllerAPI(c)
	s.client = controller.NewClient(userConn)
	s.AddCleanup(func(*gc.C) { s.client.Close() })
}

func (s *ControllerSuite) TestWatchModelSummaries(c *gc.C) {

	watcher, err := s.client.WatchModelSummaries()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Check(watcher.Stop(), jc.ErrorIsNil)
	}()

	summaries, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(summaries, jc.DeepEquals, []params.ModelAbstract{
		{
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:       "controller",
			Admins:     []string{"admin"},
			Cloud:      "dummy",
			Region:     "dummy-region",
			Credential: "dummy/admin/cred",
			Status:     "green",
		},
	})
}

func (s *ControllerSuite) TestWatchAllModelSummaries(c *gc.C) {

	watcher, err := s.client.WatchAllModelSummaries()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Check(watcher.Stop(), jc.ErrorIsNil)
	}()

	summaries, err := watcher.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(summaries, jc.DeepEquals, []params.ModelAbstract{
		{
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:       "controller",
			Admins:     []string{"admin"},
			Cloud:      "dummy",
			Region:     "dummy-region",
			Credential: "dummy/admin/cred",
			Status:     "green",
		},
	})
}
