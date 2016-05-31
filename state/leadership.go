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
	"github.com/juju/juju/state/workers"
)

func removeLeadershipSettingsOp(serviceId string) txn.Op {
	return removeSettingsOp(leadershipSettingsKey(serviceId))
}

func leadershipSettingsKey(serviceId string) string {
	return fmt.Sprintf("a#%s#leader", serviceId)
}

// LeadershipClaimer returns a leadership.Claimer for units and services in the
// state's model.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return leadershipClaimer{st.workers.LeadershipManager()}
}

// LeadershipChecker returns a leadership.Checker for units and services in the
// state's model.
func (st *State) LeadershipChecker() leadership.Checker {
	return leadershipChecker{st.workers.LeadershipManager()}
}

// HackLeadership stops the state's internal leadership manager to prevent it
// from interfering with apiserver shutdown.
func (st *State) HackLeadership() {
	// TODO(fwereade): 2015-08-07 lp:1482634
	// obviously, this should not exist: it's a quick hack to address lp:1481368 in
	// 1.24.4, and should be quickly replaced with something that isn't so heinous.
	//
	// But.
	//
	// I *believe* that what it'll take to fix this is to extract the mongo-session-
	// opening from state.Open, so we can create a mongosessioner Manifold on which
	// state, leadership, watching, tools storage, etc etc etc can all independently
	// depend. (Each dependency would/should have a separate session so they can
	// close them all on their own schedule, without panics -- but the failure of
	// the shared component should successfully goose them all into shutting down,
	// in parallel, of their own accord.)
	st.workers.Kill()
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
	manager workers.LeaseManager
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
	manager workers.LeaseManager
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
