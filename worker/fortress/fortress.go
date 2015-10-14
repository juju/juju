// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"
)

// fortress coordinates between clients that access it as a Guard and as a Guest.
type fortress struct {
	tomb         tomb.Tomb
	guardTickets chan guardTicket
	guestTickets chan guestTicket
}

// newFortress returns a new, locked, fortress. The caller is responsible for
// ensuring it somehow gets Kill()ed, and for handling any error returned by
// Wait().
func newFortress() *fortress {
	f := &fortress{
		guardTickets: make(chan guardTicket),
		guestTickets: make(chan guestTicket),
	}
	go func() {
		defer f.tomb.Done()
		f.tomb.Kill(f.loop())
	}()
	return f
}

// Kill is part of the worker.Worker interface.
func (f *fortress) Kill() {
	f.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (f *fortress) Wait() error {
	return f.tomb.Wait()
}

// Unlock is part of the Guard interface.
func (f *fortress) Unlock() error {
	return f.allowGuests(true, nil)
}

// Lockdown is part of the Guard interface.
func (f *fortress) Lockdown(abort Abort) error {
	return f.allowGuests(false, abort)
}

// Visit is part of the Guest interface.
func (f *fortress) Visit(visit Visit, abort Abort) error {
	result := make(chan error)
	select {
	case <-f.tomb.Dying():
		return errors.New("fortress worker shutting down")
	case <-abort:
		return ErrAborted
	case f.guestTickets <- guestTicket{visit, result}:
		return <-result
	}
}

// allowGuests communicates Guard-interface requests to the main loop.
func (f *fortress) allowGuests(allowGuests bool, abort Abort) error {
	result := make(chan error)
	select {
	case <-f.tomb.Dying():
		return errors.New("fortress worker shutting down")
	case f.guardTickets <- guardTicket{allowGuests, abort, result}:
		return <-result
	}
}

// loop waits for a Guard to unlock the fortress, and then runs visit funcs in
// parallel until a Guard locks it down again; at which point, it waits for all
// outstanding visits to complete, and reverts to its original state.
func (f *fortress) loop() error {
	var active sync.WaitGroup
	defer active.Wait()

	// guestTickets will be set on Unlock and cleared at the start of Lockdown.
	var guestTickets <-chan guestTicket
	for {
		select {
		case <-f.tomb.Dying():
			return tomb.ErrDying
		case ticket := <-guestTickets:
			active.Add(1)
			go ticket.complete(active.Done)
		case ticket := <-f.guardTickets:
			// guard ticket requests are idempotent; it's not worth building
			// the extra mechanism needed to (1) complain about abuse but
			// (2) remain comprehensible and functional in the face of aborted
			// Lockdowns.
			if ticket.allowGuests {
				guestTickets = f.guestTickets
			} else {
				guestTickets = nil
			}
			go ticket.complete(active.Wait)
		}
	}
}

// guardTicket communicates between the Guard interface and the main loop.
type guardTicket struct {
	allowGuests bool
	abort       Abort
	result      chan<- error
}

// complete unconditionally sends a single value on ticket.result; either nil
// (when the desired state is reached) or ErrAborted (when the ticket's Abort
// is closed). It should be called on its own goroutine.
func (ticket guardTicket) complete(waitLockedDown func()) {
	var result error
	defer func() {
		ticket.result <- result
	}()

	done := make(chan struct{})
	go func() {
		// If we're locking down, we should wait for all Visits to complete.
		// If not, Visits are already being accepted and we're already done.
		if !ticket.allowGuests {
			waitLockedDown()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ticket.abort:
		result = ErrAborted
	}
}

// guestTicket communicates between the Guest interface and the main loop.
type guestTicket struct {
	visit  Visit
	result chan<- error
}

// complete unconditionally sends any error returned from the Visit func, then
// calls the finished func. It should be called on its own goroutine.
func (ticket guestTicket) complete(finished func()) {
	defer finished()
	ticket.result <- ticket.visit()
}
