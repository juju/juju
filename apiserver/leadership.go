// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
)

// leadershipChecker implements leadership.Checker by wrapping a lease.Checker.
type leadershipChecker struct {
	checker lease.Checker
}

// LeadershipCheck is part of the leadership.Checker interface.
func (m leadershipChecker) LeadershipCheck(applicationName, unitName string) leadership.Token {
	token := m.checker.Token(applicationName, unitName)
	return leadershipToken{
		applicationName: applicationName,
		unitName:        unitName,
		token:           token,
	}
}

// leadershipToken implements leadership.Token by wrapping a lease.Token.
type leadershipToken struct {
	applicationName string
	unitName        string
	token           lease.Token
}

// Check is part of the leadership.Token interface.
func (t leadershipToken) Check() error {
	err := t.token.Check()
	if errors.Cause(err) == lease.ErrNotHeld {
		return leadership.NewNotLeaderError(t.unitName, t.applicationName)
	}
	return errors.Trace(err)
}

// leadershipClaimer implements leadership.Claimer by wrapping a lease.Claimer.
type leadershipClaimer struct {
	claimer lease.Claimer
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (m leadershipClaimer) ClaimLeadership(applicationName, unitName string, duration time.Duration) error {
	err := m.claimer.Claim(applicationName, unitName, duration)
	if errors.Cause(err) == lease.ErrClaimDenied {
		return leadership.ErrClaimDenied
	}
	return errors.Trace(err)
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (m leadershipClaimer) BlockUntilLeadershipReleased(applicationName string, cancel <-chan struct{}) error {
	err := m.claimer.WaitUntilExpired(applicationName, cancel)
	if errors.Cause(err) == lease.ErrWaitCancelled {
		return leadership.ErrBlockCancelled
	}
	return errors.Trace(err)
}

// leadershipRevoker implements leadership.Revoker by wrapping a lease.Revoker.
type leadershipRevoker struct {
	claimer lease.Revoker
}

// RevokeLeadership is part of the leadership.Claimer interface.
func (m leadershipRevoker) RevokeLeadership(applicationName, unitName string) error {
	err := m.claimer.Revoke(applicationName, unitName)
	if errors.Cause(err) == lease.ErrNotHeld {
		return leadership.ErrClaimNotHeld
	}
	return errors.Trace(err)
}

// leadershipPinner implements leadership.Pinner by wrapping a lease.Pinner.
type leadershipPinner struct {
	pinner lease.Pinner
}

// PinLeadership (leadership.Pinner) pins the lease
// for the input application and entity.
func (m leadershipPinner) PinLeadership(applicationName string, entity string) error {
	return errors.Trace(m.pinner.Pin(applicationName, entity))
}

// UnpinLeadership (leadership.Pinner) unpins the lease
// for the input application and entity.
func (m leadershipPinner) UnpinLeadership(applicationName string, entity string) error {
	return errors.Trace(m.pinner.Unpin(applicationName, entity))
}

// PinnedLeadership (leadership.Pinner) returns applications for which
// leadership is pinned, along with the entities requiring the
// pinned behaviour.
func (m leadershipPinner) PinnedLeadership() (map[string][]string, error) {
	pinned, err := m.pinner.Pinned()
	return pinned, errors.Trace(err)
}

// leadershipReader implements leadership.Reader by wrapping a lease.Reader.
type leadershipReader struct {
	reader lease.Reader
}

// Leaders (leadership.Reader) returns all application leaders in the
// current model.
func (r leadershipReader) Leaders() (map[string]string, error) {
	leaders, err := r.reader.Leases()
	return leaders, errors.Trace(err)
}
