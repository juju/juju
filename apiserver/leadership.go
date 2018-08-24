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
func (m leadershipChecker) LeadershipCheck(applicationname, unitName string) leadership.Token {
	token := m.checker.Token(applicationname, unitName)
	return leadershipToken{
		applicationname: applicationname,
		unitName:        unitName,
		token:           token,
	}
}

// leadershipToken implements leadership.Token by wrapping a lease.Token.
type leadershipToken struct {
	applicationname string
	unitName        string
	token           lease.Token
}

// Check is part of the leadership.Token interface.
func (t leadershipToken) Check(out interface{}) error {
	err := t.token.Check(out)
	if errors.Cause(err) == lease.ErrNotHeld {
		return errors.Errorf("%q is not leader of %q", t.unitName, t.applicationname)
	}
	return errors.Trace(err)
}

// leadershipClaimer implements leadership.Claimer by wrappping a lease.Claimer.
type leadershipClaimer struct {
	claimer lease.Claimer
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (m leadershipClaimer) ClaimLeadership(applicationname, unitName string, duration time.Duration) error {
	err := m.claimer.Claim(applicationname, unitName, duration)
	if errors.Cause(err) == lease.ErrClaimDenied {
		return leadership.ErrClaimDenied
	}
	return errors.Trace(err)
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (m leadershipClaimer) BlockUntilLeadershipReleased(applicationname string, cancel <-chan struct{}) error {
	err := m.claimer.WaitUntilExpired(applicationname, cancel)
	if errors.Cause(err) == lease.ErrWaitCancelled {
		return leadership.ErrBlockCancelled
	}
	return errors.Trace(err)
}
