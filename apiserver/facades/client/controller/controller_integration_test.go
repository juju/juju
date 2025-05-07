// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"

	"github.com/juju/tc"

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

var _ = tc.Suite(&ControllerIntegrationSuite{})

func (s *ControllerIntegrationSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	api := s.OpenControllerAPI(c)
	s.client = controller.NewClient(api)
	s.AddCleanup(func(*tc.C) { s.client.Close() })
}

func (s *ControllerIntegrationSuite) TestWatchModelSummaries(c *tc.C) {
	c.Skip("TODO (alvin) - reimplement when facade moved off of mongo")
	// TODO(dqlite) - implement me
	watcher, err := s.client.WatchModelSummaries(context.Background())
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.NotNil)
}

func (s *ControllerIntegrationSuite) TestWatchAllModelSummaries(c *tc.C) {
	c.Skip("TODO (alvin) - reimplement when facade moved off of mongo")
	// TODO(dqlite) - implement me
	watcher, err := s.client.WatchAllModelSummaries(context.Background())
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.NotNil)
}
