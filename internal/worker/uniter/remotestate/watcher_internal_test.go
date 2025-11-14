// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type WatcherInternalSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&WatcherInternalSuite{})

func (s *WatcherInternalSuite) TestDeleteSecretSignal(c *gc.C) {
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
	c.Assert(w.current.DeletedSecretRevisions, gc.HasLen, 0)
	c.Assert(w.current.ObsoleteSecretRevisions, jc.DeepEquals, map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666},
	})
}
