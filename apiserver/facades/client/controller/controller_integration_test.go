// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/juju/testing"
)

// This suite only exists because no user facing calls exercise
// the WatchModelSummaries or WatchAllModelSummaries.
// The primary caller is the JAAS dashboard which uses the javascript
// library. It is expected that JIMM will call these methods using
// the Go API.

type ControllerIntegrationSuite struct {
	testing.ApiServerSuite
	client *controller.Client
}

var _ = gc.Suite(&ControllerIntegrationSuite{})

func (s *ControllerIntegrationSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	api := s.OpenControllerAPI(c)
	s.client = controller.NewClient(api)
	s.AddCleanup(func(*gc.C) { s.client.Close() })
}

func (s *ControllerIntegrationSuite) TestWatchModelSummaries(c *gc.C) {

	// TODO(dqlite) - implement me
	watcher, err := s.client.WatchModelSummaries()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.NotNil)
	//c.Assert(err, jc.ErrorIsNil)
	//defer func() {
	//	c.Check(watcher.Stop(), jc.ErrorIsNil)
	//}()
	//
	//summaries, err := watcher.Next()
	//c.Assert(err, jc.ErrorIsNil)
	//
	//c.Assert(summaries, jc.DeepEquals, []params.ModelAbstract{
	//	{
	//		UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	//		Name:       "controller",
	//		Admins:     []string{"admin"},
	//		Cloud:      "dummy",
	//		Region:     "dummy-region",
	//		Credential: "dummy/admin/default",
	//		Status:     "green",
	//	},
	//})
}

func (s *ControllerIntegrationSuite) TestWatchAllModelSummaries(c *gc.C) {

	// TODO(dqlite) - implement me
	watcher, err := s.client.WatchAllModelSummaries()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.NotNil)
	//defer func() {
	//	c.Check(watcher.Stop(), jc.ErrorIsNil)
	//}()
	//
	//summaries, err := watcher.Next()
	//c.Assert(err, jc.ErrorIsNil)
	//
	//c.Assert(summaries, jc.DeepEquals, []params.ModelAbstract{
	//	{
	//		UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	//		Name:       "controller",
	//		Admins:     []string{"admin"},
	//		Cloud:      "dummy",
	//		Region:     "dummy-region",
	//		Credential: "dummy/admin/default",
	//		Status:     "green",
	//	},
	//})
}
