// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
)

func removeLeadershipSettingsOp(applicationId string) txn.Op {
	return removeSettingsOp(settingsC, leadershipSettingsKey(applicationId))
}

func leadershipSettingsKey(applicationId string) string {
	return fmt.Sprintf("a#%s#leader", applicationId)
}

// LeadershipClaimer returns a leadership.Claimer for units and applications in the
// state's model.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return leadershipClaimer{
		lazyLeaseClaimer{func() (lease.Claimer, error) {
			manager := st.workers.leadershipManager()
			return manager.Claimer(applicationLeadershipNamespace, st.modelUUID())
		}},
	}
}

// LeadershipChecker returns a leadership.Checker for units and applications in the
// state's model.
func (st *State) LeadershipChecker() leadership.Checker {
	return leadershipChecker{
		lazyLeaseChecker{func() (lease.Checker, error) {
			manager := st.workers.leadershipManager()
			return manager.Checker(applicationLeadershipNamespace, st.modelUUID())
		}},
	}
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
func (t leadershipToken) Check(out interface{}) error {
	err := t.token.Check(out)
	if errors.Cause(err) == lease.ErrNotHeld {
		return errors.Errorf("%q is not leader of %q", t.unitName, t.applicationName)
	}
	return errors.Trace(err)
}

// leadershipClaimer implements leadership.Claimer by wrapping a lease.Claimer.
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
