// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

// MachineWorkerConfig holds the configuration for a MachineWorker.
type MachineWorkerConfig struct {
	// MachineID is the ID of the machine this worker manages.
	MachineID string

	// Machine provides operations on the machine.
	Machine MachineAPI

	// Broker provides provisioning operations.
	Broker InstanceBroker

	// InstanceInfoSetter registers instance information.
	InstanceInfoSetter InstanceInfoSetter

	// Semaphore limits concurrent provider API calls.
	Semaphore ProviderSemaphore

	// Logger for logging.
	Logger Logger

	// EventsChan receives events from the main loop.
	EventsChan <-chan MachineEvent

	// RequestChan sends requests to the main loop.
	RequestChan chan<- WorkerRequest

	// DistributionGroup is the list of machine IDs in the same distribution group.
	DistributionGroup []string

	// Constraints are the zone constraints from provisioning info.
	Constraints []string

	// MaxRetries is the maximum number of retry attempts for provisioning.
	MaxRetries int

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration
}

// Validate returns an error if the config is invalid.
func (c MachineWorkerConfig) Validate() error {
	if c.MachineID == "" {
		return errors.NotValidf("empty MachineID")
	}
	if c.Machine == nil {
		return errors.NotValidf("nil Machine")
	}
	if c.Broker == nil {
		return errors.NotValidf("nil Broker")
	}
	if c.InstanceInfoSetter == nil {
		return errors.NotValidf("nil InstanceInfoSetter")
	}
	if c.Semaphore == nil {
		return errors.NotValidf("nil Semaphore")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.EventsChan == nil {
		return errors.NotValidf("nil EventsChan")
	}
	if c.RequestChan == nil {
		return errors.NotValidf("nil RequestChan")
	}
	if c.MaxRetries < 0 {
		return errors.NotValidf("negative MaxRetries")
	}
	return nil
}

// MachineWorker manages the lifecycle of a single machine using an FSM.
// It receives all events through a single channel from the main loop.
type MachineWorker struct {
	catacomb catacomb.Catacomb
	config   MachineWorkerConfig

	// FSM manages state transitions.
	fsm *FSM

	// Instance data.
	instanceID string
	zoneName   string
	retryCount int
	lastError  error

	// Retry timer for scheduling retry attempts after failures.
	retryTimer *time.Timer
}

// NewMachineWorker creates a new MachineWorker.
func NewMachineWorker(config MachineWorkerConfig) (*MachineWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &MachineWorker{
		config:     config,
		fsm:        NewFSM(),
		instanceID: config.Machine.InstanceID(),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "machine-worker-" + config.MachineID,
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill implements worker.Worker.
func (w *MachineWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *MachineWorker) Wait() error {
	return w.catacomb.Wait()
}

// transitionTo attempts to transition to the target state.
// It logs the transition and returns an error if invalid.
func (w *MachineWorker) transitionTo(ctx context.Context, target State) error {
	from := w.fsm.State()
	if err := w.fsm.TransitionTo(target); err != nil {
		w.config.Logger.Errorf(ctx, "machine %s invalid state transition: %v", w.config.MachineID, err)
		return errors.Trace(err)
	}
	w.config.Logger.Debugf(ctx, "machine %s transitioned from %s to %s", w.config.MachineID, from, target)
	return nil
}

// loop is the main event loop for the machine worker.
func (w *MachineWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	w.config.Logger.Debugf(ctx, "machine worker %s starting in state %s", w.config.MachineID, w.fsm.State())

	defer w.stopRetryTimer()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case event, ok := <-w.config.EventsChan:
			if !ok {
				return errors.New("events channel closed")
			}

			w.config.Logger.Tracef(ctx, "machine %s received event %s in state %s",
				w.config.MachineID, event.Type, w.fsm.State())

			if err := w.handleEvent(ctx, event); err != nil {
				return errors.Trace(err)
			}

			if w.fsm.IsTerminal() {
				w.config.Logger.Infof(ctx, "machine worker %s reached terminal state", w.config.MachineID)
				return nil // Clean exit.
			}

		case <-w.retryTimerChan():
			w.config.Logger.Debugf(ctx, "machine %s retry timer fired in state %s",
				w.config.MachineID, w.fsm.State())

			if err := w.handleRetryTimer(ctx); err != nil {
				return errors.Trace(err)
			}

			if w.fsm.IsTerminal() {
				w.config.Logger.Infof(ctx, "machine worker %s reached terminal state", w.config.MachineID)
				return nil // Clean exit.
			}
		}
	}
}

// retryTimerChan returns the retry timer's channel, or nil if no timer is active.
// Reading from a nil channel blocks forever, effectively disabling this select case.
func (w *MachineWorker) retryTimerChan() <-chan time.Time {
	if w.retryTimer == nil {
		return nil
	}
	return w.retryTimer.C
}

// stopRetryTimer stops the retry timer if it's active.
func (w *MachineWorker) stopRetryTimer() {
	if w.retryTimer != nil {
		w.retryTimer.Stop()
		w.retryTimer = nil
	}
}

// scheduleRetry schedules a retry attempt after the configured delay.
func (w *MachineWorker) scheduleRetry(ctx context.Context) {
	w.stopRetryTimer() // Cancel any existing timer.
	w.retryTimer = time.NewTimer(w.config.RetryDelay)
	w.config.Logger.Debugf(ctx, "machine %s scheduled retry in %v", w.config.MachineID, w.config.RetryDelay)
}

// handleRetryTimer handles the retry timer firing.
func (w *MachineWorker) handleRetryTimer(ctx context.Context) error {
	w.retryTimer = nil // Timer has fired, clear it.

	// Only retry if we're in Pending state (waiting for retry).
	if w.fsm.State() != StatePending {
		w.config.Logger.Debugf(ctx, "machine %s ignoring retry timer in state %s",
			w.config.MachineID, w.fsm.State())
		return nil
	}

	// Transition to RequestingZone and request a zone.
	if err := w.transitionTo(ctx, StateRequestingZone); err != nil {
		return err
	}
	return w.requestZone(ctx)
}

// handleEvent processes an event and performs state transitions.
func (w *MachineWorker) handleEvent(ctx context.Context, event MachineEvent) error {
	switch event.Type {
	case EventLifeChanged:
		return w.handleLifeChange(ctx, event.Life)
	case EventZoneAssigned:
		return w.handleZoneAssigned(ctx, event.Zone)
	case EventZoneRequestFailed:
		return w.handleZoneRequestFailed(ctx, event.ZoneError)
	default:
		w.config.Logger.Warningf(ctx, "machine %s received unknown event type %d", w.config.MachineID, event.Type)
		return nil
	}
}

// handleLifeChange handles lifecycle changes from the main loop.
func (w *MachineWorker) handleLifeChange(ctx context.Context, newLife life.Value) error {
	w.config.Logger.Debugf(ctx, "machine %s handling life change to %s in state %s",
		w.config.MachineID, newLife, w.fsm.State())

	switch w.fsm.State() {
	case StatePending:
		// Cancel any pending retry timer since we're handling a life event.
		w.stopRetryTimer()

		if newLife == life.Dead {
			if err := w.transitionTo(ctx, StateRemoving); err != nil {
				return err
			}
			return w.doRemove(ctx)
		}
		// life.Alive - check if already provisioned.
		if w.instanceID != "" {
			w.config.Logger.Debugf(ctx, "machine %s already has instance %s, transitioning to Running",
				w.config.MachineID, w.instanceID)
			return w.transitionTo(ctx, StateRunning)
		}
		// Not provisioned yet, start provisioning.
		if err := w.transitionTo(ctx, StateRequestingZone); err != nil {
			return err
		}
		return w.requestZone(ctx)

	case StateRequestingZone:
		if newLife == life.Dead {
			w.cancelZoneRequest(ctx)
			if err := w.transitionTo(ctx, StateRemoving); err != nil {
				return err
			}
			return w.doRemove(ctx)
		}
		// Still alive, waiting for zone response.

	case StateProvisioning:
		// Provisioning is a blocking operation. Life events are queued
		// and processed after doProvision() returns. The event will be
		// handled when we return to the event loop with a new state.
		// Note: This is handled naturally by the event queue - no special flag needed.

	case StateRunning:
		if newLife == life.Dead {
			if w.config.Machine.KeepInstance() {
				w.config.Logger.Infof(ctx, "machine %s is dead with keep-instance=true, removing without stopping",
					w.config.MachineID)
				if err := w.transitionTo(ctx, StateRemoving); err != nil {
					return err
				}
				return w.doRemove(ctx)
			}
			if err := w.transitionTo(ctx, StateStopping); err != nil {
				return err
			}
			return w.doStop(ctx)
		}

	case StateStopping, StateRemoving:
		// Already in terminal path, ignore life changes.

	case StateComplete, StateError:
		// Terminal state, nothing to do.
	}

	return nil
}

// handleZoneAssigned handles successful zone assignment.
func (w *MachineWorker) handleZoneAssigned(ctx context.Context, zone string) error {
	if w.fsm.State() != StateRequestingZone {
		// Stale response, ignore.
		w.config.Logger.Debugf(ctx, "machine %s ignoring stale zone assignment in state %s",
			w.config.MachineID, w.fsm.State())
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s assigned to zone %s", w.config.MachineID, zone)
	w.zoneName = zone
	if err := w.transitionTo(ctx, StateProvisioning); err != nil {
		return err
	}
	return w.doProvision(ctx)
}

// handleZoneRequestFailed handles zone request failure.
func (w *MachineWorker) handleZoneRequestFailed(ctx context.Context, err error) error {
	if w.fsm.State() != StateRequestingZone {
		// Stale response, ignore.
		w.config.Logger.Debugf(ctx, "machine %s ignoring stale zone request failure in state %s",
			w.config.MachineID, w.fsm.State())
		return nil
	}

	w.lastError = err
	w.retryCount++

	w.config.Logger.Warningf(ctx, "machine %s zone request failed (attempt %d/%d): %v",
		w.config.MachineID, w.retryCount, w.config.MaxRetries+1, err)

	if w.retryCount > w.config.MaxRetries {
		w.config.Logger.Errorf(ctx, "machine %s zone request retries exhausted", w.config.MachineID)
		if setErr := w.setProvisioningError(ctx, err); setErr != nil {
			w.config.Logger.Errorf(ctx, "failed to set provisioning error: %v", setErr)
		}
		return w.transitionTo(ctx, StateError)
	}

	// Go back to Pending and schedule a retry.
	if err := w.transitionTo(ctx, StatePending); err != nil {
		return err
	}
	w.scheduleRetry(ctx)
	return nil
}

// requestZone sends a zone request to the main loop.
func (w *MachineWorker) requestZone(ctx context.Context) error {
	req := NewZoneRequest(w.config.MachineID, w.config.DistributionGroup, w.config.Constraints)

	select {
	case w.config.RequestChan <- req:
		w.config.Logger.Debugf(ctx, "machine %s sent zone request", w.config.MachineID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// cancelZoneRequest cancels a pending zone request.
func (w *MachineWorker) cancelZoneRequest(ctx context.Context) {
	req := NewCancelZoneRequest(w.config.MachineID)

	select {
	case w.config.RequestChan <- req:
		w.config.Logger.Debugf(ctx, "machine %s sent cancel zone request", w.config.MachineID)
	case <-ctx.Done():
		w.config.Logger.Warningf(ctx, "machine %s failed to send cancel zone request: context done", w.config.MachineID)
	}
}

// doProvision performs StartInstance + SetInstanceInfo as a single operation.
func (w *MachineWorker) doProvision(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s starting provisioning in zone %s", w.config.MachineID, w.zoneName)

	// Acquire semaphore.
	if err := w.config.Semaphore.Acquire(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		return errors.Trace(err)
	}
	defer w.config.Semaphore.Release()

	// Call StartInstance.
	params := StartInstanceParams{
		MachineID:        w.config.MachineID,
		AvailabilityZone: w.zoneName,
	}

	result, err := w.config.Broker.StartInstance(ctx, params)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}

		w.notifyProvisionFailed(ctx, err)

		// Retry or fail.
		w.lastError = err
		w.retryCount++

		w.config.Logger.Warningf(ctx, "machine %s StartInstance failed (attempt %d/%d): %v",
			w.config.MachineID, w.retryCount, w.config.MaxRetries+1, err)

		if w.retryCount > w.config.MaxRetries {
			w.config.Logger.Errorf(ctx, "machine %s provisioning retries exhausted", w.config.MachineID)
			if setErr := w.setProvisioningError(ctx, err); setErr != nil {
				w.config.Logger.Errorf(ctx, "failed to set provisioning error: %v", setErr)
			}
			return w.transitionTo(ctx, StateError)
		}

		// Go back to Pending and schedule a retry.
		w.zoneName = ""
		if err := w.transitionTo(ctx, StatePending); err != nil {
			return err
		}
		w.scheduleRetry(ctx)
		return nil
	}

	// StartInstance succeeded.
	w.instanceID = result.InstanceID
	w.config.Logger.Infof(ctx, "machine %s started instance %s", w.config.MachineID, w.instanceID)

	// Call SetInstanceInfo.
	// NOTE: There is a window between instance creation (above) and database
	// registration (below) where a failure could leave an orphan instance.
	// This must be minimized and carefully handled. If SetInstanceInfo fails,
	// we roll back by stopping the instance.
	err = w.config.InstanceInfoSetter.SetInstanceInfo(ctx, w.config.MachineID, w.instanceID, w.zoneName)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Even on cancellation, we need to clean up the instance we created.
			w.config.Logger.Warningf(ctx, "context cancelled during SetInstanceInfo, cleaning up instance %s", w.instanceID)
		}

		// Registration failed - rollback by stopping the instance.
		w.config.Logger.Warningf(ctx, "machine %s SetInstanceInfo failed, rolling back: %v", w.config.MachineID, err)

		stopErr := w.config.Broker.StopInstances(ctx, w.instanceID)
		if stopErr != nil {
			w.config.Logger.Errorf(ctx, "machine %s rollback StopInstances failed: %v", w.config.MachineID, stopErr)
		}

		w.notifyProvisionFailed(ctx, err)

		// Clear instance state and retry.
		w.instanceID = ""
		w.zoneName = ""
		w.retryCount++

		if w.retryCount > w.config.MaxRetries {
			w.config.Logger.Errorf(ctx, "machine %s provisioning retries exhausted after SetInstanceInfo failure", w.config.MachineID)
			if setErr := w.setProvisioningError(ctx, err); setErr != nil {
				w.config.Logger.Errorf(ctx, "failed to set provisioning error: %v", setErr)
			}
			return w.transitionTo(ctx, StateError)
		}

		// Go back to Pending and schedule a retry.
		if err := w.transitionTo(ctx, StatePending); err != nil {
			return err
		}
		w.scheduleRetry(ctx)
		return nil
	}

	// Success - notify AZ Coordinator.
	w.notifyProvisionSuccess(ctx)

	w.config.Logger.Infof(ctx, "machine %s provisioning complete, instance %s in zone %s",
		w.config.MachineID, w.instanceID, w.zoneName)

	return w.transitionTo(ctx, StateRunning)
}

// notifyProvisionFailed sends a failure notification to the main loop.
func (w *MachineWorker) notifyProvisionFailed(ctx context.Context, err error) {
	req := NewProvisionCompleteRequest(w.config.MachineID, "", w.zoneName, false, err)

	select {
	case w.config.RequestChan <- req:
	case <-ctx.Done():
		w.config.Logger.Warningf(ctx, "machine %s failed to send provision failure notification", w.config.MachineID)
	}
}

// notifyProvisionSuccess sends a success notification to the main loop.
func (w *MachineWorker) notifyProvisionSuccess(ctx context.Context) {
	req := NewProvisionCompleteRequest(w.config.MachineID, w.instanceID, w.zoneName, true, nil)

	select {
	case w.config.RequestChan <- req:
	case <-ctx.Done():
		w.config.Logger.Warningf(ctx, "machine %s failed to send provision success notification", w.config.MachineID)
	}
}

// setProvisioningError sets the machine's instance status to ProvisioningError.
func (w *MachineWorker) setProvisioningError(ctx context.Context, err error) error {
	msg := err.Error()
	if setErr := w.config.Machine.SetInstanceStatus(ctx, status.ProvisioningError, msg); setErr != nil {
		return errors.Trace(setErr)
	}
	w.config.Logger.Infof(ctx, "machine %s marked as ProvisioningError after %d retries: %v",
		w.config.MachineID, w.retryCount, err)
	return nil
}

// doStop stops the instance.
func (w *MachineWorker) doStop(ctx context.Context) error {
	if w.instanceID == "" {
		// No instance to stop, go directly to removing.
		w.config.Logger.Debugf(ctx, "machine %s has no instance to stop", w.config.MachineID)
		if err := w.transitionTo(ctx, StateRemoving); err != nil {
			return err
		}
		return w.doRemove(ctx)
	}

	w.config.Logger.Infof(ctx, "machine %s stopping instance %s", w.config.MachineID, w.instanceID)

	// Acquire semaphore.
	if err := w.config.Semaphore.Acquire(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		return errors.Trace(err)
	}
	defer w.config.Semaphore.Release()

	err := w.config.Broker.StopInstances(ctx, w.instanceID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		// Retry - stay in Stopping state.
		w.lastError = err
		w.config.Logger.Warningf(ctx, "machine %s StopInstances failed, will retry: %v", w.config.MachineID, err)
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s instance %s stopped", w.config.MachineID, w.instanceID)

	if err := w.transitionTo(ctx, StateRemoving); err != nil {
		return err
	}
	return w.doRemove(ctx)
}

// doRemove removes the machine record.
func (w *MachineWorker) doRemove(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s removing machine record", w.config.MachineID)

	err := w.config.Machine.MarkForRemoval(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		// Retry - stay in Removing state.
		w.lastError = err
		w.config.Logger.Warningf(ctx, "machine %s MarkForRemoval failed, will retry: %v", w.config.MachineID, err)
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s removed", w.config.MachineID)

	return w.transitionTo(ctx, StateComplete)
}
