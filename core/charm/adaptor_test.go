// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/deployment/charm"
)

type adaptorSuite struct{}

func TestAdaptorSuite(t *testing.T) {
	tc.Run(t, &adaptorSuite{})
}

// TestCharmInfoAdaptorActions ensures that the charm info adaptor
// returns the actions from the essential metadata, rather than nil.
// This is critical for deploying charms from a repository, where the
// adaptor is used to pass essential metadata (including actions) to
// the application service for storage at deploy time.
func (s *adaptorSuite) TestCharmInfoAdaptorActions(c *tc.C) {
	actions := &charm.Actions{
		ActionSpecs: map[string]charm.ActionSpec{
			"backup": {
				Description: "Take a backup",
				Parallel:    true,
			},
		},
	}

	adaptor := NewCharmInfoAdaptor(EssentialMetadata{
		Actions: actions,
	})

	c.Check(adaptor.Actions(), tc.DeepEquals, actions)
}

// TestCharmInfoAdaptorActionsNil ensures that the adaptor returns nil
// when no actions are present in the essential metadata.
func (s *adaptorSuite) TestCharmInfoAdaptorActionsNil(c *tc.C) {
	adaptor := NewCharmInfoAdaptor(EssentialMetadata{})

	c.Check(adaptor.Actions(), tc.IsNil)
}

// TestCharmInfoAdaptorActionsEmpty ensures that the adaptor returns an
// empty (non-nil) actions set when the essential metadata contains one.
func (s *adaptorSuite) TestCharmInfoAdaptorActionsEmpty(c *tc.C) {
	adaptor := NewCharmInfoAdaptor(EssentialMetadata{
		Actions: charm.NewActions(),
	})

	got := adaptor.Actions()
	c.Assert(got, tc.NotNil)
	c.Check(got.ActionSpecs, tc.HasLen, 0)
}
