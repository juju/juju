// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

// Manager
type Manager interface {

	// ClaimLeadership
	ClaimLeadership(serviceName, unitName string, duration time.Duration) error

	// CheckLeadership
	CheckLeadership(serviceName, unitName string) (Token, error)

	// WatchLeaderless
	WatchLeaderless(serviceName string) (NotifyWatcher, error)
}

// Token
type Token interface {

	//AssertOps
	AssertOps() []txn.Op
}
