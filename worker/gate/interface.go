// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// package gate provides a mechanism by which independent workers can wait for
// one another to finish a task, without introducing explicit dependencies
// between those workers.
package gate

// Unlocker is used to unlock a shared gate.
type Unlocker interface {
	Unlock()
}

// Waiter is used to wait for a shared gate to be unlocked.
type Waiter interface {
	Unlocked() <-chan struct{}
}

// AlreadyUnlocked is a Waiter that always reports its gate to be unlocked.
type AlreadyUnlocked struct{}

// Unlocked is part of the Waiter interface.
func (AlreadyUnlocked) Unlocked() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
