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

	"github.com/juju/juju/worker/uniter/container"
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
			err := n.processNotice(containerName, notice, pebbleClient)
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

func (n *pebbleNoticer) processNotice(containerName string, notice *client.Notice, pebbleClient PebbleClient) error {
	var eventType container.WorkloadEventType
	var event container.WorkloadEvent
	switch notice.Type {
	case client.CustomNotice:
		eventType = container.CustomNoticeEvent
		event = container.WorkloadEvent{
			Type:         eventType,
			WorkloadName: containerName,
			NoticeID:     notice.ID,
			NoticeType:   string(notice.Type),
			NoticeKey:    notice.Key,
		}
	case client.ChangeUpdateNotice:
		data := notice.LastData
		kind := data["kind"]
		// Since the charm is triggering most Pebble changes, there is little
		// value in feeding events based on those changes back to the charm.
		// As such, we do not send events for all change-updated notices (see
		// OP045 for more background). However, perform-check and recover-check
		// change kinds reflect changes in the workload state, so are useful to
		// charms.
		if kind != "perform-check" && kind != "recover-check" {
			n.logger.Debugf("container %q: ignoring %s notice, kind %s", containerName, notice.Type, kind)
			return nil
		}
		// We always look for the final status (Done, Error), because the status
		// might have changed since the notice was updated and now. We know that
		// the notice for the change will never update from Done/Error to
		// anything else, so cannot miss the change entirely.
		chg, err := pebbleClient.Change(notice.Key)
		if err != nil {
			// Couldn't fetch change associated with notice, likely because it's
			// been pruned. Pebble prunes changes when they're 7 days old or
			// there's more than 500 total changes, so this may happen if the
			// check has been in the same state (perform or recover) for a long
			// time and then changes state. In this case, proceed and assume the
			// change was completed (Error for perform-check, Done for recover-check).
			n.logger.Debugf("container %q: %s notice, could not fetch change %q: %v",
				containerName, notice.Type, notice.Key, err)
		}

		// Although we determine that a check has reached the failure threshold
		// (or is again succeeding) via a Pebble notice, we consider that an
		// implementation detail and provide specific hook types. We have a pair
		// of hooks as this reflects a change of (workload) state, rather than
		// a change of data. See OP046 for more background.
		switch {
		case kind == "perform-check" && (chg == nil || chg.Status == "Error"):
			eventType = container.CheckFailedEvent
		case kind == "recover-check" && (chg == nil || chg.Status == "Done"):
			eventType = container.CheckRecoveredEvent
		default:
			chgStatus := "<unknown>"
			if chg != nil {
				chgStatus = chg.Status
			}
			n.logger.Debugf("container %q: ignoring %s, status %s", containerName, kind, chgStatus)
			return nil
		}
		event = container.WorkloadEvent{
			Type:         eventType,
			WorkloadName: containerName,
			CheckName:    data["check-name"],
		}
	default:
		n.logger.Debugf("container %q: ignoring %s notice", containerName, notice.Type)
		return nil
	}

	n.logger.Debugf("container %q: processing %s notice, key %q", containerName, notice.Type, notice.Key)

	errChan := make(chan error, 1)
	eventID := n.workloadEvents.AddWorkloadEvent(event, func(err error) {
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
