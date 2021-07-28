package globalclockupdater

import (
	"time"

	"github.com/hashicorp/raft"
)

type updater struct {
	raft *raft.Raft
}

// Advance implements globalClock.Updater by applying
// a clock advance operation to the raft member.
func (u *updater) Advance(duration time.Duration, stop <-chan struct{}) error {
	return nil
}
