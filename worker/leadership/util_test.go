// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/testing"

	"github.com/juju/juju/leadership"
)

type StubLeadershipManager struct {
	leadership.LeadershipManager
	*testing.Stub
	releases chan struct{}
}

func (stub *StubLeadershipManager) ClaimLeadership(serviceName, unitName string, duration time.Duration) error {
	stub.MethodCall(stub, "ClaimLeadership", serviceName, unitName, duration)
	return stub.NextErr()
}

func (stub *StubLeadershipManager) BlockUntilLeadershipReleased(serviceName string) error {
	stub.MethodCall(stub, "BlockUntilLeadershipReleased", serviceName)
	<-stub.releases
	return stub.NextErr()
}
