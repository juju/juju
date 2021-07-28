package globalclockupdater

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/juju/core/raftlease"
)

const applyTimeout = 5 * time.Second

// updater implements globalClock.Updater by applying
// a clock advance operation to the raft member.
type updater struct {
	raft     *raft.Raft
	clock    raftlease.ReadOnlyClock
	prevTime time.Time
}

func newUpdater(r *raft.Raft, clock raftlease.ReadOnlyClock) *updater {
	return &updater{
		raft:     r,
		clock:    clock,
		prevTime: clock.GlobalTime(),
	}
}

// Advance applies the clock advance operation to Raft if it is the current
// leader. This updater is recruited by a worker that depends on being run on
// the same machine/container as the leader, so
func (u *updater) Advance(duration time.Duration, stop <-chan struct{}) error {
	newTime := u.prevTime.Add(duration)

	// Apply the operation.

	u.prevTime = newTime
	return nil
}
