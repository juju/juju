// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package globalclockupdater -destination raft_test.go github.com/juju/juju/worker/globalclockupdater RaftApplier,Logger,Sleeper
//go:generate go run github.com/golang/mock/mockgen -package globalclockupdater -destination raftlease_test.go github.com/juju/juju/core/raftlease NotifyTarget,ReadOnlyClock,FSMResponse
//go:generate go run github.com/golang/mock/mockgen -package globalclockupdater -destination rafterror_test.go github.com/hashicorp/raft ApplyFuture

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
