package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

func removeLeadershipSettingsOp(serviceId string) txn.Op {
	return removeSettingsOp(leadershipSettingsKey(serviceId))
}

func leadershipSettingsKey(serviceId string) string {
	return fmt.Sprintf("s#%s#leader", serviceId)
}

// LeadershipClaimer returns a leadership.Claimer for units and services in the
// state's model.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return leadershipClaimer{st.leadershipManager}
}

// LeadershipChecker returns a leadership.Checker for units and services in the
// state's model.
func (st *State) LeadershipChecker() leadership.Checker {
	return leadershipChecker{st.leadershipManager}
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
	st.leadershipManager.Kill()
	st.singularManager.Kill()
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
// service names, and holders are unit names.
type leadershipSecretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (leadershipSecretary) CheckLease(name string) error {
	if !names.IsValidService(name) {
		return errors.NewNotValid(nil, "not a service name")
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

// leadershipChecker implements leadership.Checker by wrapping a lease.Manager.
type leadershipChecker struct {
	manager *lease.Manager
}

// LeadershipCheck is part of the leadership.Checker interface.
func (m leadershipChecker) LeadershipCheck(serviceName, unitName string) leadership.Token {
	token := m.manager.Token(serviceName, unitName)
	return leadershipToken{
		serviceName: serviceName,
		unitName:    unitName,
		token:       token,
	}
}

// leadershipToken implements leadership.Token by wrapping a corelease.Token.
type leadershipToken struct {
	serviceName string
	unitName    string
	token       corelease.Token
}

// Check is part of the leadership.Token interface.
func (t leadershipToken) Check(out interface{}) error {
	err := t.token.Check(out)
	if errors.Cause(err) == corelease.ErrNotHeld {
		return errors.Errorf("%q is not leader of %q", t.unitName, t.serviceName)
	}
	return errors.Trace(err)
}

// leadershipClaimer implements leadership.Claimer by wrappping a lease.Manager.
type leadershipClaimer struct {
	manager *lease.Manager
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (m leadershipClaimer) ClaimLeadership(serviceName, unitName string, duration time.Duration) error {
	err := m.manager.Claim(serviceName, unitName, duration)
	if errors.Cause(err) == corelease.ErrClaimDenied {
		return leadership.ErrClaimDenied
	}
	return errors.Trace(err)
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (m leadershipClaimer) BlockUntilLeadershipReleased(serviceName string) error {
	err := m.manager.WaitUntilExpired(serviceName)
	return errors.Trace(err)
}
