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

func NewLeadershipManager(leaseMgr LeadershipLeaseManager) *Manager {
	return &Manager{
		leaseMgr: leaseMgr,
	}
}

type Manager struct {
	leaseMgr LeadershipLeaseManager
}

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

func (m *Manager) ReleaseLeadership(sid, uid string) error {
	return m.leaseMgr.ReleaseLease(leadershipNamespace(sid), uid)
}

func (m *Manager) BlockUntilLeadershipReleased(serviceId string) error {
	notifier := m.leaseMgr.LeaseReleasedNotifier(leadershipNamespace(serviceId))
	<-notifier
	return nil
}

func leadershipNamespace(serviceId string) string {
	return serviceId + leadershipNamespaceSuffix
}
