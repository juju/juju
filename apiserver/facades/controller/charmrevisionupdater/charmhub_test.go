// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(benhoyt) - add test with some charm resources

package charmrevisionupdater_test

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

type charmhubSuite struct{}

var _ = gc.Suite(&charmhubSuite{})

func newMockCharmhubClient(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
	charms := map[string]hubCharm{
		"charm-1": {
			id:       "charm-1",
			name:     "mysql",
			revision: 23,
			version:  "23",
		},
		"charm-2": {
			id:       "charm-2",
			name:     "postgresql",
			revision: 42,
			version:  "42",
		},
	}
	return &mockCharmhubAPI{charms: charms, metadata: metadata}, nil
}

type hubCharm struct {
	id        string
	name      string
	revision  int
	resources []transport.ResourceRevision
	version   string
}

type mockCharmhubAPI struct {
	charms   map[string]hubCharm
	metadata map[string]string
}

func (h *mockCharmhubAPI) Refresh(_ context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
	// Sanity check that metadata headers are present
	if h.metadata["model_uuid"] == "" {
		return nil, errors.Errorf("model metadata not present")
	}

	request, err := config.Build()
	if err != nil {
		return nil, err
	}
	responses := make([]transport.RefreshResponse, len(request.Context))
	for i, context := range request.Context {
		action := request.Actions[i]
		if action.Action != "refresh" {
			return nil, errors.Errorf("unexpected action %q", action.Action)
		}
		if *action.ID != context.ID {
			return nil, errors.Errorf("action ID %q doesn't match context ID %q", *action.ID, context.ID)
		}
		charm, ok := h.charms[context.ID]
		if !ok {
			return nil, errors.Errorf("charm ID %q not found", context.ID)
		}
		response := transport.RefreshResponse{
			Entity: transport.RefreshEntity{
				CreatedAt: time.Now(),
				ID:        context.ID,
				Name:      charm.name,
				Resources: charm.resources,
				Revision:  charm.revision,
				Version:   charm.version,
			},
			EffectiveChannel: context.TrackingChannel,
			ID:               context.ID,
			InstanceKey:      context.InstanceKey,
			Name:             charm.name,
			Result:           "refresh",
		}
		responses[i] = response
	}
	return responses, nil
}

func (s *charmhubSuite) TestUpdateRevisionsOutOfDate(c *gc.C) {
	state := newMockState(newMockResources())
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newMockCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	state.addApplication("ch", "mysql", "charm-1", "app-1", 22)
	state.addApplication("ch", "postgresql", "charm-2", "app-2", 41)

	c.Assert(state.charmPlaceholders, jc.DeepEquals, set.NewStrings())

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	expected := set.NewStrings("ch:mysql-23", "ch:postgresql-42")
	c.Assert(state.charmPlaceholders, jc.DeepEquals, expected)
}

func (s *charmhubSuite) TestUpdateRevisionsUpToDate(c *gc.C) {
	state := newMockState(newMockResources())
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newMockCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	state.addApplication("ch", "postgresql", "charm-2", "app-1", 42)

	c.Assert(state.charmPlaceholders, jc.DeepEquals, set.NewStrings())

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// AddCharmPlaceholder will be called even when the revision isn't updating.
	expected := set.NewStrings("ch:postgresql-42")
	c.Assert(state.charmPlaceholders, jc.DeepEquals, expected)
}
