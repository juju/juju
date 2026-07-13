// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate

// Unlocker is used to unlock a shared gate.
type Unlocker interface {
	Unlock()
}

// Waiter is used to wait for a shared gate to be unlocked.
type Waiter interface {
	Unlocked() <-chan struct{}
	IsUnlocked() bool
}

// Lock combines the Waiter and Unlocker interfaces.
type Lock interface {
	Waiter
	Unlocker
}

// AlreadyUnlocked is a Lock that always reports its gate to be unlocked.
// Unlock is a no-op.
type AlreadyUnlocked struct{}

// Unlock is a no-op; the gate is already unlocked.
func (AlreadyUnlocked) Unlock() {}

// Unlocked is part of the Waiter interface.
func (AlreadyUnlocked) Unlocked() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// IsUnlocked is part of the Waiter interface.
func (AlreadyUnlocked) IsUnlocked() bool {
	return true
}
