// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"errors"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	internalerrors "github.com/juju/juju/internal/errors"
)

const (
	// DefaultSignalTimeout is the default timeout for signalling a change to a
	// subscriber.
	// Failure to consume the changes within this time will result in the
	// subscriber being unsubscribed.
	DefaultSignalTimeout = time.Second * 10

	subscriptionClosed = internalerrors.ConstError("subscription closed")
)

type requestSubscription struct {
	opts   []changestream.SubscriptionOption
	result chan requestSubscriptionResult
}

type requestSubscriptionResult struct {
	sub *subscription
	err error
}

// subscription represents a subscriber in the event queue. It holds a tomb, so
// that we can tie the lifecycle of a subscription to the event queue.
type subscription struct {
	tomb tomb.Tomb
	id   uint64

	topics        map[string]struct{}
	changes       chan ChangeSet
	unsubscribeFn func()
}

func newSubscription(id uint64, unsubscribeFn func()) *subscription {
	sub := &subscription{
		id:            id,
		changes:       make(chan ChangeSet),
		topics:        make(map[string]struct{}),
		unsubscribeFn: unsubscribeFn,
	}

	sub.tomb.Go(sub.loop)

	return sub
}

// Unsubscribe removes the subscription from the event queue asynchronously.
func (s *subscription) Unsubscribe() {
	s.unsubscribeFn()
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
	ctx, cancel := context.WithTimeout(ctx, DefaultSignalTimeout)
	defer cancel()

	select {
	case <-s.tomb.Dying():
		return s.tomb.Err()

	case <-ctx.Done():
		// If the context was timed out, which means that nothing was pulling
		// the change off from the channel. Then in this scenario it better that
		// the listener is unsubscribed from any future events and will be
		// notified via the done channel. The listener will still have the
		// opportunity to resubscribe in the future. They're just no longer
		// par-taking in this term whilst they're unresponsive.
		err := ctx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			s.Unsubscribe()
		}
		return err

	case s.changes <- changes:
		return nil
	}
}

// close closes the active channel, which will signal to the consumer that the
// subscription is no longer active.
func (s *subscription) close() error {
	s.tomb.Kill(subscriptionClosed)
	if err := s.Wait(); err != nil && !errors.Is(err, subscriptionClosed) {
		return err
	}
	return nil
}
