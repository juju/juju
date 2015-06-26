// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/lease"
)

const (
	leadershipNamespaceSuffix = "-leadership"
)

var errWorkerStopped = errors.New("worker stopped")

// NewLeadershipManager returns a new Manager.
func NewLeadershipManager(leaseMgr LeadershipLeaseManager) *Manager {
	return &Manager{
		leaseMgr: leaseMgr,
	}
}

// Manager represents the business logic for leadership management.
type Manager struct {
	leaseMgr LeadershipLeaseManager
}

// Leader returns whether or not the given unit id is currently the
// leader for the given service ID.
func (m *Manager) Leader(sid, uid string) (bool, error) {
	tok, err := m.leaseMgr.RetrieveLease(leadershipNamespace(sid))
	if errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return tok.Id == uid, nil
}

// ClaimLeadership implements the LeadershipManager interface.
func (m *Manager) ClaimLeadership(sid, uid string, duration time.Duration) error {

	_, err := m.leaseMgr.ClaimLease(leadershipNamespace(sid), uid, duration)
	if err != nil {
		if errors.Cause(err) == lease.LeaseClaimDeniedErr {
			err = errors.Wrap(err, ErrClaimDenied)
		} else {
			err = errors.Annotate(err, "unable to make a leadership claim")
		}
	}

	return err
}

// ReleaseLeadership implements the LeadershipManager interface.
func (m *Manager) ReleaseLeadership(sid, uid string) error {
	return m.leaseMgr.ReleaseLease(leadershipNamespace(sid), uid)
}

// BlockUntilLeadershipReleased implements the LeadershipManager interface.
func (m *Manager) BlockUntilLeadershipReleased(serviceId string) error {
	notifier, err := m.leaseMgr.LeaseReleasedNotifier(leadershipNamespace(serviceId))
	if err != nil {
		return err
	}
	_, ok := <-notifier
	if !ok {
		return errWorkerStopped
	}
	return nil
}

func leadershipNamespace(serviceId string) string {
	return serviceId + leadershipNamespaceSuffix
}
