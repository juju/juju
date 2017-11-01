// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/testing"

	"github.com/juju/juju/core/leadership"
)

type StubClaimer struct {
	leadership.Claimer
	*testing.Stub
	releases chan struct{}
}

func (stub *StubClaimer) ClaimLeadership(serviceName, unitName string, duration time.Duration) error {
	stub.MethodCall(stub, "ClaimLeadership", serviceName, unitName, duration)
	return stub.NextErr()
}

func (stub *StubClaimer) BlockUntilLeadershipReleased(serviceName string, cancel <-chan struct{}) error {
	stub.MethodCall(stub, "BlockUntilLeadershipReleased", serviceName)
	select {
	case <-cancel:
		return leadership.ErrBlockCancelled
	case <-stub.releases:
	}
	return stub.NextErr()
}
