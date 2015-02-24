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
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.leadership")

// Ticket is used with Tracker to communicate leadership status back to a client.
type Ticket chan bool

// Tracker allows clients to discover current leadership status by attempting to
// claim it for themselves.
type Tracker interface {

	// ClaimLeader will cause the ticket to be closed in the near future; if true
	// is sent before it's closed, the unit's leadership is guaranteed for the
	// tracker's duration.
	ClaimLeader(ticket Ticket)
}

// TrackerWorker embeds the Tracker and worker.Worker interfaces.
type TrackerWorker interface {
	worker.Worker
	Tracker
}

type tracker struct {
	tomb        tomb.Tomb
	leadership  leadership.LeadershipManager
	unitName    string
	serviceName string
	duration    time.Duration
	isMinion    bool

	claimLease   chan struct{}
	renewLease   <-chan time.Time
	claimTickets chan Ticket
}

// NewTrackerWorker returns a TrackerWorker that attempts to claim and retain
// service leadership for the supplied unit. It will claim leadership for the
// supplied duration, and once it has it it will renew leadership every time
// the duration is half elapsed.
func NewTrackerWorker(tag names.UnitTag, leadership leadership.LeadershipManager, duration time.Duration) TrackerWorker {
	unitName := tag.Id()
	serviceName, _ := names.UnitService(unitName)
	t := &tracker{
		unitName:     unitName,
		serviceName:  serviceName,
		leadership:   leadership,
		duration:     duration,
		claimTickets: make(chan Ticket),
	}
	go func() {
		defer t.tomb.Done()
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

// ClaimLeader is part of the Tracker interface.
func (t *tracker) ClaimLeader(ticket Ticket) {
	t.send(ticket, t.claimTickets)
}

func (t *tracker) loop() error {
	logger.Infof("making initial claim")
	if err := t.refresh(); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case <-t.claimLease:
			logger.Infof("claiming lease")
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case <-t.renewLease:
			logger.Infof("renewing lease")
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		case ticket := <-t.claimTickets:
			logger.Infof("got claim request")
			if err := t.resolveClaim(ticket); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// refresh makes a leadership request, and updates tracker state to conform to
// latest known reality.
func (t *tracker) refresh() error {
	logger.Infof("checking leadership...")
	untilTime := time.Now().Add(t.duration)
	err := t.leadership.ClaimLeadership(t.serviceName, t.unitName, t.duration)
	switch {
	case err == nil:
		t.setLeader(untilTime)
	case errors.Cause(err) == leadership.ErrClaimDenied:
		t.setMinion()
	default:
		return errors.Annotatef(err, "leadership failure")
	}
	return nil
}

// setLeader arranges for lease renewal.
func (t *tracker) setLeader(untilTime time.Time) {
	logger.Infof("leadership confirmed until %s", untilTime)
	renewTime := untilTime.Add(-(t.duration / 2))
	logger.Infof("will renew at %s", renewTime)
	t.isMinion = false
	t.claimLease = nil
	t.renewLease = time.After(renewTime.Sub(time.Now()))
}

// setLeader arranges for lease acquisition when there's an opportunity.
func (t *tracker) setMinion() {
	logger.Infof("leadership denied")
	t.isMinion = true
	t.renewLease = nil
	if t.claimLease == nil {
		t.claimLease = make(chan struct{})
		go func() {
			logger.Infof("waiting for leadership release")
			t.leadership.BlockUntilLeadershipReleased(t.serviceName)
			close(t.claimLease)
		}()
	}
}

// resolveClaim will send true on the supplied channel if leadership can be
// successfully verified, and will always close it whether or not it sent.
func (t *tracker) resolveClaim(ticket Ticket) error {
	logger.Infof("checking leadership ticket...")
	defer close(ticket)
	if !t.isMinion {
		// Last time we looked, we were leader.
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case <-t.renewLease:
			logger.Infof("renewing lease")
			t.renewLease = nil
			if err := t.refresh(); err != nil {
				return errors.Trace(err)
			}
		default:
			logger.Infof("still leader")
		}
	}
	if t.isMinion {
		logger.Infof("not leader")
		return nil
	}
	logger.Infof("confirming leadership ticket")
	return t.confirm(ticket)
}

func (t *tracker) send(ticket Ticket, ch chan Ticket) {
	select {
	case <-t.tomb.Dying():
		close(ticket)
	case ch <- ticket:
	}
}

func (t *tracker) confirm(ticket Ticket) error {
	select {
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case ticket <- true:
	}
	return nil
}
