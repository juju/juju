// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type WatcherInternalSuite struct {
	testing.BaseSuite
}

func TestWatcherInternalSuite(t *stdtesting.T) {
	tc.Run(t, &WatcherInternalSuite{})
}

func (s *WatcherInternalSuite) TestDeleteSecretSignal(c *tc.C) {
	w := RemoteStateWatcher{
		current: Snapshot{
			ObsoleteSecretRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665, 666},
				"secret:777e2mr0ui3e8a215n4g": {777},
			},
			DeletedSecretRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665},
			},
		},
	}
	w.RemoveSecretsCompleted(map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {665},
		"secret:777e2mr0ui3e8a215n4g": {},
	})
	c.Assert(w.current.DeletedSecretRevisions, tc.HasLen, 0)
	c.Assert(w.current.ObsoleteSecretRevisions, tc.DeepEquals, map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666},
	})
}
