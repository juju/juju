// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package notifyproxy

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/clock"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

const (
	// BufferSize is the amount of notifications to enqueue before dropping the
	// oldest.
	BufferSize = 512

	// ClaimedTimeout defines how long we should wait for a claimed timeout
	// response.
	ClaimedTimeout = time.Second * 15

	// ExpirationsTimeout defines how long we should wait for an expirations
	// timeout response. This is longer than a claimed timeout, because we are
	// processing multiple records rather than one.
	ExpirationsTimeout = time.Second * 30
)

// NotifyType defines a notification type.
type NotifyType string

const (
	// Claimed defines the claimed notification type.
	Claimed NotifyType = "claimed"

	// Expirations defines the expirations notification type.
	Expirations NotifyType = "expirations"
)

// NotificationProxy allows notifications to be sent via a proxy, rather than
// directly to state, allowing the decoupling of state to a given worker.
type NotificationProxy interface {
	// Notifications returns a channel of notifications from a given notify
	// target.
	Notifications() <-chan []Notification
}

// Notification defines a typed notification sent from the proxy.
type Notification interface {
	// Type returns the notification type.
	Type() NotifyType

	// ErrorResponse is used to notify the proxy call of any potential errors
	// when sending.
	ErrorResponse(error)
}

// ClaimedNote returns the information associated with a claimed request.
type ClaimedNote struct {
	Key      lease.Key
	Holder   string
	response func(error)
}

// Type returns the notification type.
func (ClaimedNote) Type() NotifyType {
	return Claimed
}

// ErrorResponse is used to notify the proxy call of any potential errors
// when sending.
func (n ClaimedNote) ErrorResponse(err error) {
	if n.response != nil {
		n.response(err)
	}
}

// ExpirationsNote returns the information
// associated with an expirations request.
type ExpirationsNote struct {
	Expirations []raftlease.Expired
	response    func(error)
}

func (ExpirationsNote) Type() NotifyType {
	return Expirations
}

// ErrorResponse is used to notify the proxy call of any potential errors
// when sending.
func (n ExpirationsNote) ErrorResponse(err error) {
	if n.response != nil {
		n.response(err)
	}
}

type NotifyProxy struct {
	tomb     *tomb.Tomb
	in       chan Notification
	out      chan []Notification
	blocking bool
	clock    clock.Clock
}

// NewBlocking creates a new NotifyProxy, that blocks waiting for a response
// back from the queries.
func NewBlocking(clock clock.Clock) *NotifyProxy {
	return newProxy(true, clock)
}

// NewNonBlocking creates a new NotifyProxy that doesn't block waiting for a
// response.
func NewNonBlocking(clock clock.Clock) *NotifyProxy {
	return newProxy(false, clock)
}

func newProxy(blocking bool, clock clock.Clock) *NotifyProxy {
	proxy := &NotifyProxy{
		tomb:     new(tomb.Tomb),
		in:       make(chan Notification),
		out:      make(chan []Notification),
		blocking: blocking,
		clock:    clock,
	}
	proxy.tomb.Go(proxy.loop)
	return proxy
}

// Claimed will be called when a new lease has been claimed.
func (p *NotifyProxy) Claimed(key lease.Key, holder string) error {
	var (
		response func(error)
		err      = make(chan error, 1)
	)
	if p.blocking {
		response = func(e error) {
			err <- e
		}
	} else {
		close(err)
	}

	select {
	case <-p.tomb.Dying():
		return tomb.ErrDying
	case <-p.tomb.Dead():
		return p.tomb.Err()
	case p.in <- ClaimedNote{
		Key:      key,
		Holder:   holder,
		response: response,
	}:
	}

	select {
	case e := <-err:
		return errors.Trace(e)
	case <-time.After(ClaimedTimeout):
		return errors.Errorf("claimed timed out for key %v with holder %s", key, holder)
	}
}

// Expirations will be called when a set of existing leases have expired.
func (p *NotifyProxy) Expirations(expirations []raftlease.Expired) error {
	var (
		response func(error)
		err      = make(chan error, 1)
	)
	if p.blocking {
		response = func(e error) {
			err <- e
		}
	} else {
		close(err)
	}

	select {
	case <-p.tomb.Dying():
		return tomb.ErrDying
	case <-p.tomb.Dead():
		return p.tomb.Err()
	case p.in <- ExpirationsNote{
		Expirations: expirations,
		response:    response,
	}:
	}

	select {
	case e := <-err:
		return errors.Trace(e)
	case <-time.After(ExpirationsTimeout):
		return errors.Errorf("expirations timed out")
	}
}

// Notifications returns a channel of notifications from a given notify
// target.
func (p *NotifyProxy) Notifications() <-chan []Notification {
	return p.out
}

// Close the NotifyProxy.
func (p *NotifyProxy) Close() error {
	p.Kill(nil)
	return p.Wait()
}

// Kill puts the tomb in a dying state for the given reason,
// closes the Dying channel, and sets Alive to false.
func (p *NotifyProxy) Kill(reason error) {
	p.tomb.Kill(reason)
}

// Wait blocks until all goroutines have finished running, and
// then returns the reason for their death.
func (p *NotifyProxy) Wait() error {
	return p.tomb.Wait()
}

func (p *NotifyProxy) loop() error {
	defer close(p.out)

	out := p.out

	var buffer []Notification
	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-p.tomb.Dead():
			return p.tomb.Err()
		case note := <-p.in:
			// This would be better a linked list, so that we can just grab
			// the tail and drop the head.
			if len(buffer) == BufferSize {
				entity := buffer[0]
				entity.ErrorResponse(errors.Errorf("timed out waiting to be processed"))

				buffer = buffer[1:]
			}

			buffer = append(buffer, note)
			out = p.out

		case out <- buffer:
			buffer = make([]Notification, 0)
			out = nil

		default:
			// If there is no work to do, pause briefly
			// so that this loop is not thrashing CPU.
			time.Sleep(5 * time.Millisecond)
		}
	}
}
