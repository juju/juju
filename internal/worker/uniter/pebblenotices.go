// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"

	"github.com/canonical/pebble/client"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/worker/uniter/container"
)

type pebbleNoticer struct {
	logger            Logger
	clock             clock.Clock
	workloadEventChan chan string
	workloadEvents    container.WorkloadEvents
	newPebbleClient   NewPebbleClientFunc

	tomb tomb.Tomb
}

// NewPebbleNoticer starts a worker that watches for Pebble notices on the
// specified containers.
func NewPebbleNoticer(
	logger Logger,
	clock clock.Clock,
	containerNames []string,
	workloadEventChan chan string,
	workloadEvents container.WorkloadEvents,
	newPebbleClient NewPebbleClientFunc,
) worker.Worker {
	if newPebbleClient == nil {
		newPebbleClient = func(config *client.Config) (PebbleClient, error) {
			return client.New(config)
		}
	}
	noticer := &pebbleNoticer{
		logger:            logger,
		clock:             clock,
		workloadEventChan: workloadEventChan,
		workloadEvents:    workloadEvents,
		newPebbleClient:   newPebbleClient,
	}
	for _, name := range containerNames {
		name := name
		noticer.tomb.Go(func() error {
			return noticer.run(name)
		})
	}
	return noticer
}

// Kill is part of the worker.Worker interface.
func (n *pebbleNoticer) Kill() {
	n.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (n *pebbleNoticer) Wait() error {
	return n.tomb.Wait()
}

func (n *pebbleNoticer) run(containerName string) (err error) {
	const (
		waitTimeout = 30 * time.Second
		errorDelay  = time.Second
	)

	n.logger.Debugf("container %q: pebbleNoticer starting", containerName)
	defer n.logger.Debugf("container %q: pebbleNoticer stopped, error %v", containerName, err)

	config := newPebbleConfig(containerName)
	pebbleClient, err := n.newPebbleClient(config)
	if err != nil {
		return errors.Trace(err)
	}
	defer pebbleClient.CloseIdleConnections()

	var after time.Time
	ctx := n.tomb.Context(nil)
	for {
		// Wait up to a timeout for new notices to arrive (also stop when
		// tomb's context is cancelled).
		options := &client.NoticesOptions{After: after}
		notices, err := pebbleClient.WaitNotices(ctx, waitTimeout, options)

		// Return early if the worker was killed.
		select {
		case <-n.tomb.Dying():
			return tomb.ErrDying
		default:
		}

		// If an error occurred, wait a bit and try again.
		if err != nil {
			var socketNotFound *client.SocketNotFoundError
			if errors.As(err, &socketNotFound) {
				// Pebble has probably not started yet -- not an error.
				n.logger.Debugf("container %q: socket %q not found, waiting %s",
					containerName, socketNotFound.Path, errorDelay)
			} else {
				n.logger.Errorf("container %q: WaitNotices error, waiting %s: %v",
					containerName, errorDelay, err)
			}
			select {
			case <-n.clock.After(errorDelay):
			case <-n.tomb.Dying():
				return tomb.ErrDying
			}
			continue
		}

		// Send any notices as Juju events.
		for _, notice := range notices {
			err := n.processNotice(containerName, notice)
			if err != nil {
				// Avoid wrapping or tracing this error, as processNotice can
				// return tomb.ErrDying, and tomb doesn't use errors.Is yet.
				return err
			}

			// Update the next "after" query time to the latest LastRepeated value.
			after = notice.LastRepeated
		}
	}
}

func (n *pebbleNoticer) processNotice(containerName string, notice *client.Notice) error {
	var eventType container.WorkloadEventType
	switch notice.Type {
	case client.CustomNotice:
		eventType = container.CustomNoticeEvent
	default:
		n.logger.Debugf("container %q: ignoring %s notice", containerName, notice.Type)
		return nil
	}

	n.logger.Debugf("container %q: processing %s notice, key %q", containerName, notice.Type, notice.Key)

	errChan := make(chan error, 1)
	eventID := n.workloadEvents.AddWorkloadEvent(container.WorkloadEvent{
		Type:         eventType,
		WorkloadName: containerName,
		NoticeID:     notice.ID,
		NoticeType:   string(notice.Type),
		NoticeKey:    notice.Key,
	}, func(err error) {
		errChan <- errors.Trace(err)
	})
	defer n.workloadEvents.RemoveWorkloadEvent(eventID)

	// Send the event to the charm!
	select {
	case n.workloadEventChan <- eventID:
	case <-n.tomb.Dying():
		return tomb.ErrDying
	}

	select {
	case err := <-errChan:
		if err != nil {
			return errors.Annotatef(err, "failed to send event for %s notice, key %q",
				notice.Type, notice.Key)
		}
	case <-n.tomb.Dying():
		return tomb.ErrDying
	}

	return nil
}
