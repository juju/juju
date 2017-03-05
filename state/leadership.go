// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

func removeLeadershipSettingsOp(applicationId string) txn.Op {
	return removeSettingsOp(settingsC, leadershipSettingsKey(applicationId))
}

func leadershipSettingsKey(applicationId string) string {
	return fmt.Sprintf("a#%s#leader", applicationId)
}

// LeadershipClaimer returns a leadership.Claimer for units and services in the
// state's model.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return leadershipClaimer{st.workers.leadershipManager()}
}

// LeadershipChecker returns a leadership.Checker for units and services in the
// state's model.
func (st *State) LeadershipChecker() leadership.Checker {
	return leadershipChecker{st.workers.leadershipManager()}
}

// buildTxnWithLeadership returns a transaction source that combines the supplied source
// with checks and asserts on the supplied token.
func buildTxnWithLeadership(buildTxn jujutxn.TransactionSource, token leadership.Token) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		var prereqs []txn.Op
		if err := token.Check(&prereqs); err != nil {
			return nil, errors.Annotatef(err, "prerequisites failed")
		}
		ops, err := buildTxn(attempt)
		if err == jujutxn.ErrNoOperations {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return append(prereqs, ops...), nil
	}
}

// leadershipSecretary implements lease.Secretary; it checks that leases are
// application names, and holders are unit names.
type leadershipSecretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (leadershipSecretary) CheckLease(name string) error {
	if !names.IsValidApplication(name) {
		return errors.NewNotValid(nil, "not an application name")
	}
	return nil
}

// CheckHolder is part of the lease.Secretary interface.
func (leadershipSecretary) CheckHolder(name string) error {
	if !names.IsValidUnit(name) {
		return errors.NewNotValid(nil, "not a unit name")
	}
	return nil
}

// CheckDuration is part of the lease.Secretary interface.
func (leadershipSecretary) CheckDuration(duration time.Duration) error {
	if duration <= 0 {
		return errors.NewNotValid(nil, "non-positive")
	}
	return nil
}

// leadershipChecker implements leadership.Checker by wrapping a LeaseManager.
type leadershipChecker struct {
	manager *lease.Manager
}

// LeadershipCheck is part of the leadership.Checker interface.
func (m leadershipChecker) LeadershipCheck(applicationname, unitName string) leadership.Token {
	token := m.manager.Token(applicationname, unitName)
	return leadershipToken{
		applicationname: applicationname,
		unitName:        unitName,
		token:           token,
	}
}

// leadershipToken implements leadership.Token by wrapping a corelease.Token.
type leadershipToken struct {
	applicationname string
	unitName        string
	token           corelease.Token
}

// Check is part of the leadership.Token interface.
func (t leadershipToken) Check(out interface{}) error {
	err := t.token.Check(out)
	if errors.Cause(err) == corelease.ErrNotHeld {
		return errors.Errorf("%q is not leader of %q", t.unitName, t.applicationname)
	}
	return errors.Trace(err)
}

// leadershipClaimer implements leadership.Claimer by wrappping a LeaseManager.
type leadershipClaimer struct {
	manager *lease.Manager
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (m leadershipClaimer) ClaimLeadership(applicationname, unitName string, duration time.Duration) error {
	err := m.manager.Claim(applicationname, unitName, duration)
	if errors.Cause(err) == corelease.ErrClaimDenied {
		return leadership.ErrClaimDenied
	}
	return errors.Trace(err)
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (m leadershipClaimer) BlockUntilLeadershipReleased(applicationname string) error {
	err := m.manager.WaitUntilExpired(applicationname)
	return errors.Trace(err)
}
