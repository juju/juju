// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/leadership"
)

var logger = loggo.GetLogger("juju.worker.leadership")

// tracker implements TrackerWorker.
type tracker struct {
	tomb        tomb.Tomb
	leadership  leadership.LeadershipManager
	unitName    string
	serviceName string
	duration    time.Duration
	isMinion    bool

	claimLease   chan struct{}
	renewLease   <-chan time.Time
	claimTickets chan chan bool
	waitTickets  chan chan bool
	waiting      []chan bool
}

// NewTrackerWorker returns a TrackerWorker that attempts to claim and retain
// service leadership for the supplied unit. It will claim leadership for twice
// the supplied duration, and once it's leader it will renew leadership every
// time the duration elapses.
// Thus, successful leadership claims on the resulting Tracker will guarantee
// leadership for the duration supplied here without generating additional calls
// to the supplied manager (which may very well be on the other side of a
// network connection).
func NewTrackerWorker(tag names.UnitTag, leadership leadership.LeadershipManager, duration time.Duration) TrackerWorker {
	unitName := tag.Id()
	serviceName, _ := names.UnitService(unitName)
	t := &tracker{
		unitName:     unitName,
		serviceName:  serviceName,
		leadership:   leadership,
		duration:     duration,
		claimTickets: make(chan chan bool),
		waitTickets:  make(chan chan bool),
	}
	go func() {
		defer t.tomb.Done()
		defer func() {
			for _, ticketCh := range t.waiting {
				close(ticketCh)
			}
		}()
		t.tomb.Kill(t.loop())
	}()
	return t
}

// Kill is part of the worker.Worker interface.
func (t *tracker) Kill() {
	t.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *tracker) Wait() error {
	return t.tomb.Wait()
}

// ServiceName is part of the Tracker interface.
func (t *tracker) ServiceName() string {
	return t.serviceName
}

// ClaimDuration is part of the Tracker interface.
func (t *tracker) ClaimDuration() time.Duration {
	return t.duration
}

// ClaimLeader is part of the Tracker interface.
func (t *tracker) ClaimLeader() Ticket {
	return t.submit(t.claimTickets)
}

// WaitLeader is part of the Tracker interface.
func (t *tracker) WaitLeader() Ticket {
	return t.submit(t.waitTickets)
}

func (t *tracker) loop() error {
	logger.Infof("%s making initial claim for %s leadership", t.unitName, t.serviceName)
	if err := t.refresh(); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case <-t.claimLease:
			logger.Infof("%s claiming lease for %s leadership", t.unitName, t.serviceName)
			t.claimLease = nil
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case <-t.renewLease:
			logger.Infof("%s renewing lease for %s leadership", t.unitName, t.serviceName)
			t.renewLease = nil
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case ticketCh := <-t.claimTickets:
			logger.Infof("%s got claim request for %s leadership", t.unitName, t.serviceName)
			if err := t.resolveClaim(ticketCh); err != nil {
				return errors.Trace(err)
			}
		case ticketCh := <-t.waitTickets:
			logger.Infof("%s got wait request for %s leadership", t.unitName, t.serviceName)
			if err := t.resolveWait(ticketCh); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// refresh makes a leadership request, and updates tracker state to conform to
// latest known reality.
func (t *tracker) refresh() error {
	logger.Infof("checking %s for %s leadership", t.unitName, t.serviceName)
	leaseDuration := 2 * t.duration
	untilTime := time.Now().Add(leaseDuration)
	err := t.leadership.ClaimLeadership(t.serviceName, t.unitName, leaseDuration)
	switch {
	case err == nil:
		return t.setLeader(untilTime)
	case errors.Cause(err) == leadership.ErrClaimDenied:
		return t.setMinion()
	}
	return errors.Annotatef(err, "leadership failure")
}

// setLeader arranges for lease renewal.
func (t *tracker) setLeader(untilTime time.Time) error {
	logger.Infof("%s confirmed for %s leadership until %s", t.unitName, t.serviceName, untilTime)
	renewTime := untilTime.Add(-t.duration)
	logger.Infof("%s will renew %s leadership at %s", t.unitName, t.serviceName, renewTime)
	t.isMinion = false
	t.claimLease = nil
	t.renewLease = time.After(renewTime.Sub(time.Now()))

	for len(t.waiting) > 0 {
		var ticketCh chan bool
		ticketCh, t.waiting = t.waiting[0], t.waiting[1:]
		defer close(ticketCh)
		if err := t.sendTrue(ticketCh); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// setMinion arranges for lease acquisition when there's an opportunity.
func (t *tracker) setMinion() error {
	logger.Infof("%s leadership for %s denied", t.serviceName, t.unitName)
	t.isMinion = true
	t.renewLease = nil
	if t.claimLease == nil {
		t.claimLease = make(chan struct{})
		go func() {
			defer close(t.claimLease)
			logger.Infof("%s waiting for %s leadership release", t.unitName, t.serviceName)
			err := t.leadership.BlockUntilLeadershipReleased(t.serviceName)
			if err != nil {
				logger.Warningf("error while %s waiting for %s leadership release: %v", t.unitName, t.serviceName, err)
			}
			// We don't need to do anything else with the error, because we just
			// close the claimLease channel and trigger a leadership claim on the
			// main loop; if anything's gone seriously wrong we'll find out right
			// away and shut down anyway. (And if this goroutine outlives the
			// tracker, it keeps it around as a zombie, but I don't see a way
			// around that...)
		}()
	}
	return nil
}

// isLeader returns true if leadership is guaranteed for the tracker's duration.
func (t *tracker) isLeader() (bool, error) {
	if !t.isMinion {
		// Last time we looked, we were leader.
		select {
		case <-t.tomb.Dying():
			return false, tomb.ErrDying
		case <-t.renewLease:
			logger.Infof("%s renewing lease for %s leadership", t.unitName, t.serviceName)
			t.renewLease = nil
			if err := t.refresh(); err != nil {
				return false, errors.Trace(err)
			}
		default:
			logger.Infof("%s still has %s leadership", t.unitName, t.serviceName)
		}
	}
	return !t.isMinion, nil
}

// resolveClaim will send true on the supplied channel if leadership can be
// successfully verified, and will always close it whether or not it sent.
func (t *tracker) resolveClaim(ticketCh chan bool) error {
	logger.Infof("resolving %s leadership ticket for %s...", t.serviceName, t.unitName)
	defer close(ticketCh)
	if leader, err := t.isLeader(); err != nil {
		return errors.Trace(err)
	} else if !leader {
		logger.Infof("%s is not %s leader", t.unitName, t.serviceName)
		return nil
	}
	logger.Infof("confirming %s leadership for %s", t.serviceName, t.unitName)
	return t.sendTrue(ticketCh)
}

// resolveWait will send true on the supplied channel if leadership can be
// guaranteed for the tracker's duration. It will then close the channel. If
// leadership cannot be guaranteed, the channel is left untouched until either
// the termination of the tracker or the next invocation of setLeader; at which
// point true is sent if applicable, and the channel is closed.
func (t *tracker) resolveWait(ticketCh chan bool) error {
	var dontClose bool
	defer func() {
		if !dontClose {
			close(ticketCh)
		}
	}()

	if leader, err := t.isLeader(); err != nil {
		return errors.Trace(err)
	} else if leader {
		logger.Infof("reporting %s leadership for %s", t.serviceName, t.unitName)
		return t.sendTrue(ticketCh)
	}

	logger.Infof("waiting for %s to attain %s leadership", t.unitName, t.serviceName)
	t.waiting = append(t.waiting, ticketCh)
	dontClose = true
	return nil
}

func (t *tracker) sendTrue(ticketCh chan bool) error {
	select {
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case ticketCh <- true:
		return nil
	}
}

func (t *tracker) submit(tickets chan chan bool) Ticket {
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

// ticket is used with tracker to communicate leadership status back to a client.
type ticket struct {
	ch      chan bool
	ready   chan struct{}
	success bool
}

func (t *ticket) run() {
	defer close(t.ready)
	// This is only safe/sane because the tracker promises to close all pending
	// ticket channels when it shuts down.
	if <-t.ch {
		t.success = true
	}
}

// Ready is part of the Ticket interface.
func (t *ticket) Ready() <-chan struct{} {
	return t.ready
}

// Wait is part of the Ticket interface.
func (t *ticket) Wait() bool {
	<-t.ready
	return t.success
}
