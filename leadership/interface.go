package leadership

import (
	"time"

	"github.com/juju/errors"
)

var LeadershipClaimDeniedErr = errors.New("the leadership claim has been denied.")

type LeadershipManager interface {
	ClaimLeadership(serviceId, unitId string) (nextClaimInterval time.Duration, err error)
	ReleaseLeadership(serviceId, unitId string) (err error)
	BlockUntilLeadershipReleased(serviceId string) (err error)
}

type LeadershipLeaseManager interface {
	ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error)
	ReleaseLease(namespace, id string) (err error)
	LeaseReleasedNotifier(namespace string) (notifier <-chan struct{})
}
