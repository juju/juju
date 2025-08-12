// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/internal/errors"
)

const (
	// DefaultSignalTimeout is the default timeout for signalling a change to a
	// subscriber.
	// Failure to consume the changes within this time will result in the
	// subscriber being unsubscribed.
	DefaultSignalTimeout = time.Second * 10

	// ErrUnsubscribing is returned when a subscription is killed while
	// dispatching changes.
	ErrUnsubscribing = errors.ConstError("unsubscribing during dispatching")
)

type requestSubscription struct {
	summary string
	opts    []changestream.SubscriptionOption
	result  chan requestSubscriptionResult
}

type requestSubscriptionResult struct {
	sub *subscription
	err error
}

// subscription represents a subscriber in the event queue. It holds a tomb,
// so that we can tie the lifecycle of a subscription to the event queue.
type subscription struct {
	tomb    tomb.Tomb
	id      uint64
	summary string

	topics  map[string]struct{}
	changes chan ChangeSet

	dispatchTimeout time.Duration
}

func newSubscription(id uint64, summary string) *subscription {
	sub := &subscription{
		id:              id,
		summary:         summary,
		changes:         make(chan ChangeSet),
		topics:          make(map[string]struct{}),
		dispatchTimeout: DefaultSignalTimeout,
	}

	sub.tomb.Go(sub.loop)

	return sub
}

// Changes returns the channel that the subscription will receive events on.
func (s *subscription) Changes() <-chan []changestream.ChangeEvent {
	return s.changes
}

// Done provides a way to know from the consumer side if the underlying
// subscription has been terminated. This is useful to know if the event queue
// has been closed.
func (s *subscription) Done() <-chan struct{} {
	return s.tomb.Dying()
}

// Summary provides a summary of the subscription, which can be used for
// debugging purposes.
func (s *subscription) Summary() string {
	if s.summary != "" {
		return s.summary
	}
	return "unknown"
}

// Kill implements worker.Worker.
func (s *subscription) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (s *subscription) Wait() error {
	return s.tomb.Wait()
}

func (s *subscription) loop() error {
	<-s.tomb.Dying()
	return tomb.ErrDying
}

func (s *subscription) dispatch(ctx context.Context, changes ChangeSet) error {
	ctx, cancel := context.WithTimeout(ctx, s.dispatchTimeout)
	defer cancel()

	select {
	case <-s.tomb.Dying():
		return ErrUnsubscribing
	case <-ctx.Done():
		// If the subscriber is not consuming changes in a timely manner,
		// we will get [context.DeadlineExceeded] and kill the subscription.
		// This will cause the subscriber's loop to exit on the next pass.
		// If the context was cancelled, it means the multiplexer is dying,
		// so it is safe to kill in that case as well.
		err := ctx.Err()
		if err != nil {
			s.Kill()
		}
		return ctx.Err()
	case s.changes <- changes:
		return nil
	}
}
