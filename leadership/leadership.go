// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/lease"
)

const (
	leadershipDuration        = 30 * time.Second
	leadershipNamespaceSuffix = "-leadership"
)

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

// ClaimLeadership implements the LeadershipManager interface.
func (m *Manager) ClaimLeadership(sid, uid string) (time.Duration, error) {

	_, err := m.leaseMgr.ClaimLease(leadershipNamespace(sid), uid, leadershipDuration)
	if err != nil {
		if errors.Cause(err) == lease.LeaseClaimDeniedErr {
			err = errors.Wrap(err, LeadershipClaimDeniedErr)
		} else {
			err = errors.Annotate(err, "unable to make a leadership claim.")
		}
	}

	return leadershipDuration, err
}

// ReleaseLeadership implements the LeadershipManager interface.
func (m *Manager) ReleaseLeadership(sid, uid string) error {
	return m.leaseMgr.ReleaseLease(leadershipNamespace(sid), uid)
}

// BlockUntilLeadershipReleased implements the LeadershipManager interface.
func (m *Manager) BlockUntilLeadershipReleased(serviceId string) error {
	notifier := m.leaseMgr.LeaseReleasedNotifier(leadershipNamespace(serviceId))
	<-notifier
	return nil
}

func leadershipNamespace(serviceId string) string {
	return serviceId + leadershipNamespaceSuffix
}
