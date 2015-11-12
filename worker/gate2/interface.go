// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// package gate2 provides a mechanism by which independent workers can wait for
// one another to finish a task, without introducing explicit dependencies
// between those workers.
package gate2

// Checker is an output from the gate2 Manifold.
type Checker interface {
	// IsUnlocked returns true when the gate is unlocked.
	IsUnlocked() bool
}

// Unlocker is used to unlock a gate.
type Unlocker interface {
	// Unlock will unlock a gate. It is goroutine-safe and may be
	// called multiple times. Only the first call will have any effect
	// however.
	Unlock()

	// Unlocked returns a channel that will be closed when the gate is
	// unlocked.
	//
	// TODO(mjs) - This becomes unnecessary once the machine agent
	// dependency engine conversion is done. The returned channel is
	// necessary while we have unconverted workers that need a channel
	// to block their startup waiting for the upgrader and
	// upgrade-steps workers. Eventually, the Unlocker interface can
	// go away completely and be replaced with a simple unlock
	// functioning returned by Manifold.
	Unlocked() <-chan struct{}
}
