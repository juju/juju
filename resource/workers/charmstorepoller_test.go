// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"time"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/resource/workers"
)

type LatestCharmHandlerSuite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	store *stubDataStore
}

var _ = gc.Suite(&LatestCharmHandlerSuite{})

func (s *LatestCharmHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.store = &stubDataStore{Stub: s.stub}
}

func (s *LatestCharmHandlerSuite) TestSuccess(c *gc.C) {
	applicationID := names.NewApplicationTag("a-application")
	info := charmstore.CharmInfo{
		OriginalURL:    &charm.URL{},
		Timestamp:      time.Now().UTC(),
		LatestRevision: 2,
		LatestResources: []charmresource.Resource{
			resourcetesting.NewCharmResource(c, "spam", "<some data>"),
		},
	}
	handler := workers.NewLatestCharmHandler(s.store)

	err := handler.HandleLatest(applicationID, info)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetCharmStoreResources")
	s.stub.CheckCall(c, 0, "SetCharmStoreResources", "a-application", info.LatestResources, info.Timestamp)
}

type stubDataStore struct {
	*testing.Stub
}

func (s *stubDataStore) SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error {
	s.AddCall("SetCharmStoreResources", applicationID, info, lastPolled)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
