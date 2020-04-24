// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/leadership"
)

var logger = loggo.GetLogger("juju.worker.leadership")

type Tracker struct {
	tomb            tomb.Tomb
	claimer         leadership.Claimer
	unitName        string
	applicationName string
	clock           clock.Clock
	duration        time.Duration
	isMinion        bool

	claimLease        chan error
	renewLease        <-chan time.Time
	claimTickets      chan chan bool
	waitLeaderTickets chan chan bool
	waitMinionTickets chan chan bool
	waitingLeader     []chan bool
	waitingMinion     []chan bool
}

// NewTracker returns a *Tracker that attempts to claim and retain service
// leadership for the supplied unit. It will claim leadership for twice the
// supplied duration, and once it's leader it will renew leadership every
// time the duration elapses.
// Thus, successful leadership claims on the resulting Tracker will guarantee
// leadership for the duration supplied here without generating additional
// calls to the supplied manager (which may very well be on the other side of
// a network connection).
func NewTracker(tag names.UnitTag, claimer leadership.Claimer, clock clock.Clock, duration time.Duration) *Tracker {
	unitName := tag.Id()
	serviceName, _ := names.UnitApplication(unitName)
	t := &Tracker{
		unitName:          unitName,
		applicationName:   serviceName,
		claimer:           claimer,
		clock:             clock,
		duration:          duration,
		claimTickets:      make(chan chan bool),
		waitLeaderTickets: make(chan chan bool),
		waitMinionTickets: make(chan chan bool),
		isMinion:          true,
	}
	t.tomb.Go(func() error {
		defer func() {
			for _, ticketCh := range t.waitingLeader {
				close(ticketCh)
			}
			for _, ticketCh := range t.waitingMinion {
				close(ticketCh)
			}
		}()
		err := t.loop()
		// TODO: jam 2015-04-02 is this the most elegant way to make
		// sure we shutdown cleanly? Essentially the lowest level sees
		// that we are dying, and propagates an ErrDying up to us so
		// that we shut down, which we then are passing back into
		// Tomb.Kill().
		// Tomb.Kill() special cases the exact object ErrDying, and has
		// no idea about errors.Cause and the general errors.Trace
		// mechanisms that we use.
		// So we explicitly unwrap before calling tomb.Kill() else
		// tomb.Stop() thinks that we have a genuine error.
		switch cause := errors.Cause(err); cause {
		case tomb.ErrDying:
			err = cause
		}
		return err
	})
	return t
}

// Kill is part of the worker.Worker interface.
func (t *Tracker) Kill() {
	t.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Tracker) Wait() error {
	return t.tomb.Wait()
}

// ApplicationName is part of the leadership.Tracker interface.
func (t *Tracker) ApplicationName() string {
	return t.applicationName
}

// ClaimDuration is part of the leadership.Tracker interface.
func (t *Tracker) ClaimDuration() time.Duration {
	return t.duration
}

// ClaimLeader is part of the leadership.Tracker interface.
func (t *Tracker) ClaimLeader() leadership.Ticket {
	return t.submit(t.claimTickets)
}

// WaitLeader is part of the leadership.Tracker interface.
func (t *Tracker) WaitLeader() leadership.Ticket {
	return t.submit(t.waitLeaderTickets)
}

// WaitMinion is part of the leadership.Tracker interface.
func (t *Tracker) WaitMinion() leadership.Ticket {
	return t.submit(t.waitMinionTickets)
}

func (t *Tracker) loop() error {
	logger.Debugf("%s making initial claim for %s leadership", t.unitName, t.applicationName)
	if err := t.refresh(); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case err, ok := <-t.claimLease:
			t.claimLease = nil
			if errors.Cause(err) == leadership.ErrBlockCancelled || !ok {
				// BlockUntilLeadershipReleased was cancelled,
				// which means that the tracker is terminating.
				continue
			} else if err != nil {
				return errors.Annotatef(err,
					"error while %s waiting for %s leadership release",
					t.unitName, t.applicationName,
				)
			}
			logger.Tracef("%s claiming lease for %s leadership", t.unitName, t.applicationName)
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case <-t.renewLease:
			logger.Tracef("%s renewing lease for %s leadership", t.unitName, t.applicationName)
			t.renewLease = nil
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case ticketCh := <-t.claimTickets:
			logger.Tracef("%s got claim request for %s leadership", t.unitName, t.applicationName)
			if err := t.resolveClaim(ticketCh); err != nil {
				return errors.Trace(err)
			}
		case ticketCh := <-t.waitLeaderTickets:
			logger.Tracef("%s got wait request for %s leadership", t.unitName, t.applicationName)
			if err := t.resolveWaitLeader(ticketCh); err != nil {
				return errors.Trace(err)
			}
		case ticketCh := <-t.waitMinionTickets:
			logger.Tracef("%s got wait request for %s leadership loss", t.unitName, t.applicationName)
			if err := t.resolveWaitMinion(ticketCh); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// refresh makes a leadership request, and updates Tracker state to conform to
// latest known reality.
func (t *Tracker) refresh() error {
	logger.Tracef("checking %s for %s leadership", t.unitName, t.applicationName)
	leaseDuration := 2 * t.duration
	untilTime := t.clock.Now().Add(leaseDuration)
	err := t.claimer.ClaimLeadership(t.applicationName, t.unitName, leaseDuration)
	switch {
	case err == nil:
		return t.setLeader(untilTime)
	case errors.Cause(err) == leadership.ErrClaimDenied:
		return t.setMinion()
	}
	return errors.Annotatef(err, "leadership failure")
}

// setLeader arranges for lease renewal.
func (t *Tracker) setLeader(untilTime time.Time) error {
	if t.isMinion {
		// If we were a minion, we're now the leader, so we can record the transition.
		logger.Infof("%s promoted to leadership of %s", t.unitName, t.applicationName)
	}
	logger.Tracef("%s confirmed for %s leadership until %s", t.unitName, t.applicationName, untilTime)
	renewTime := untilTime.Add(-t.duration)
	logger.Tracef("%s will renew %s leadership at %s", t.unitName, t.applicationName, renewTime)
	t.isMinion = false
	t.claimLease = nil
	t.renewLease = t.clock.After(renewTime.Sub(t.clock.Now()))

	for len(t.waitingLeader) > 0 {
		logger.Tracef("notifying %s ticket of impending %s leadership", t.unitName, t.applicationName)
		var ticketCh chan bool
		ticketCh, t.waitingLeader = t.waitingLeader[0], t.waitingLeader[1:]
		defer close(ticketCh)
		if err := t.sendTrue(ticketCh); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// setMinion arranges for lease acquisition when there's an opportunity.
func (t *Tracker) setMinion() error {
	logger.Infof("%s leadership for %s denied", t.applicationName, t.unitName)
	t.isMinion = true
	t.renewLease = nil
	if t.claimLease == nil {
		t.claimLease = make(chan error, 1)
		go func() {
			defer close(t.claimLease)
			logger.Debugf("%s waiting for %s leadership release", t.unitName, t.applicationName)
			err := t.claimer.BlockUntilLeadershipReleased(t.applicationName, t.tomb.Dying())
			if err != nil {
				logger.Debugf("%s waiting for %s leadership release gave err: %s", t.unitName, t.applicationName, err)
			}
			select {
			case t.claimLease <- err:
			case <-t.tomb.Dying():
			}
		}()
	}

	for len(t.waitingMinion) > 0 {
		logger.Debugf("notifying %s ticket of impending loss of %s leadership", t.unitName, t.applicationName)
		var ticketCh chan bool
		ticketCh, t.waitingMinion = t.waitingMinion[0], t.waitingMinion[1:]
		defer close(ticketCh)
		if err := t.sendTrue(ticketCh); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// isLeader returns true if leadership is guaranteed for the Tracker's duration.
func (t *Tracker) isLeader() (bool, error) {
	if !t.isMinion {
		// Last time we looked, we were leader.
		select {
		case <-t.tomb.Dying():
			return false, errors.Trace(tomb.ErrDying)
		case <-t.renewLease:
			logger.Tracef("%s renewing lease for %s leadership", t.unitName, t.applicationName)
			t.renewLease = nil
			if err := t.refresh(); err != nil {
				return false, errors.Trace(err)
			}
		default:
			logger.Tracef("%s still has %s leadership", t.unitName, t.applicationName)
		}
	}
	return !t.isMinion, nil
}

// resolveClaim will send true on the supplied channel if leadership can be
// successfully verified, and will always close it whether or not it sent.
func (t *Tracker) resolveClaim(ticketCh chan bool) error {
	logger.Tracef("resolving %s leadership ticket for %s...", t.applicationName, t.unitName)
	defer close(ticketCh)
	if leader, err := t.isLeader(); err != nil {
		return errors.Trace(err)
	} else if !leader {
		logger.Debugf("%s is not %s leader", t.unitName, t.applicationName)
		return nil
	}
	logger.Tracef("confirming %s leadership for %s", t.applicationName, t.unitName)
	return t.sendTrue(ticketCh)
}

// resolveWaitLeader will send true on the supplied channel if leadership can be
// guaranteed for the Tracker's duration. It will then close the channel. If
// leadership cannot be guaranteed, the channel is left untouched until either
// the termination of the Tracker or the next invocation of setLeader; at which
// point true is sent if applicable, and the channel is closed.
func (t *Tracker) resolveWaitLeader(ticketCh chan bool) error {
	var dontClose bool
	defer func() {
		if !dontClose {
			close(ticketCh)
		}
	}()

	if leader, err := t.isLeader(); err != nil {
		return errors.Trace(err)
	} else if leader {
		logger.Tracef("reporting %s leadership for %s", t.applicationName, t.unitName)
		return t.sendTrue(ticketCh)
	}

	logger.Tracef("waiting for %s to attain %s leadership", t.unitName, t.applicationName)
	t.waitingLeader = append(t.waitingLeader, ticketCh)
	dontClose = true
	return nil
}

// resolveWaitMinion will close the supplied channel as soon as leadership cannot
// be guaranteed beyond the Tracker's duration.
func (t *Tracker) resolveWaitMinion(ticketCh chan bool) error {
	var dontClose bool
	defer func() {
		if !dontClose {
			close(ticketCh)
		}
	}()

	if leader, err := t.isLeader(); err != nil {
		return errors.Trace(err)
	} else if leader {
		logger.Tracef("waiting for %s to lose %s leadership", t.unitName, t.applicationName)
		t.waitingMinion = append(t.waitingMinion, ticketCh)
		dontClose = true
	} else {
		logger.Tracef("reporting %s leadership loss for %s", t.applicationName, t.unitName)
	}
	return nil

}

func (t *Tracker) sendTrue(ticketCh chan bool) error {
	select {
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case ticketCh <- true:
		return nil
	}
}

func (t *Tracker) submit(tickets chan chan bool) leadership.Ticket {
	ticketCh := make(chan bool, 1)
	select {
	case <-t.tomb.Dying():
		close(ticketCh)
	case tickets <- ticketCh:
	}
	ticket := &ticket{
		ch:    ticketCh,
		ready: make(chan struct{}),
	}
	go ticket.run()
	return ticket
}

// ticket is used by Tracker to communicate leadership status back to a client.
type ticket struct {
	ch      chan bool
	ready   chan struct{}
	success bool
}

func (t *ticket) run() {
	defer close(t.ready)
	// This is only safe/sane because the Tracker promises to close all pending
	// ticket channels when it shuts down.
	if <-t.ch {
		t.success = true
	}
}

// Ready is part of the leadership.Ticket interface.
func (t *ticket) Ready() <-chan struct{} {
	return t.ready
}

// Wait is part of the leadership.Ticket interface.
func (t *ticket) Wait() bool {
	<-t.ready
	return t.success
}
