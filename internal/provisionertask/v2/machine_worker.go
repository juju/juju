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

	// MessageChan sends messages to the main loop.
	MessageChan chan<- WorkerMessage

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
	if c.MessageChan == nil {
		return errors.NotValidf("nil MessageChan")
	}
	if c.MaxRetries < 0 {
		return errors.NotValidf("negative MaxRetries")
	}
	return nil
}

// MachineWorker manages the lifecycle of a single machine using an FSM.
// It receives all events through a single channel from the main loop.
//
// The worker uses a state-entry pattern: the main loop drives the FSM by
// executing the action for the current state on every iteration, then waiting
// for an event only when the state did not change (i.e. the worker is idle or
// waiting for an external response). This guarantees that:
//
//   - Each state has exactly one discrete, testable action.
//   - Event handlers are pure state-transition functions with no side effects.
//   - Life events that arrive while an active operation is in progress (e.g.
//     during StartInstance) are safely buffered in EventsChan and processed
//     once the worker returns to an idle state.
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
func (w *MachineWorker) transitionTo(ctx context.Context, target MachineState) error {
	from := w.fsm.State()
	if err := w.fsm.TransitionTo(target); err != nil {
		w.config.Logger.Errorf(ctx, "machine %s invalid state transition: %v", w.config.MachineID, err)
		return errors.Trace(err)
	}
	w.config.Logger.Debugf(ctx, "machine %s transitioned from %s to %s", w.config.MachineID, from, target)
	return nil
}

// loop is the main event loop for the machine worker.
//
// The loop follows a state-entry pattern:
//  1. Call driveState to execute the action for the current state.
//  2. If the state changed, loop immediately (don't wait for an event) so the
//     new state's action runs without delay.
//  3. If the state did not change (idle or waiting), block on the select until
//     an event, timer, or dying signal arrives.
func (w *MachineWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	w.config.Logger.Debugf(ctx, "machine worker %s starting in state %s", w.config.MachineID, w.fsm.State())

	defer w.stopRetryTimer()

	for {
		prevState := w.fsm.State()

		if err := w.driveState(ctx); err != nil {
			return errors.Trace(err)
		}
		if w.fsm.IsTerminal() {
			w.config.Logger.Infof(ctx, "machine worker %s reached terminal state %s", w.config.MachineID, w.fsm.State())
			return nil
		}

		// If driveState changed the state, loop back immediately to run the
		// new state's action without waiting for an external event. This is
		// what keeps life events safely buffered in EventsChan while active
		// operations (Provisioning → Registering → RollingBack) chain through
		// without entering the select.
		if w.fsm.State() != prevState {
			continue
		}

		// State unchanged: the worker is idle or waiting. Block until the next
		// event or timer signal, then loop back to driveState.
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case event, ok := <-w.config.EventsChan:
			if !ok {
				return errors.New("events channel closed")
			}

			w.config.Logger.Tracef(ctx, "machine %s received event %s in state %s",
				w.config.MachineID, event.Type, w.fsm.State())

			if err := w.applyEvent(ctx, event); err != nil {
				return errors.Trace(err)
			}

		case <-w.retryTimerChan():
			w.config.Logger.Debugf(ctx, "machine %s retry timer fired in state %s",
				w.config.MachineID, w.fsm.State())

			if err := w.applyRetryTimer(ctx); err != nil {
				return errors.Trace(err)
			}
		}

		if w.fsm.IsTerminal() {
			w.config.Logger.Infof(ctx, "machine worker %s reached terminal state %s", w.config.MachineID, w.fsm.State())
			return nil
		}
	}
}

// driveState executes the entry action for the current state.
// Idle states (Pending, Running, Complete, Error) have no action.
// Active states execute one discrete operation and transition on completion.
func (w *MachineWorker) driveState(ctx context.Context) error {
	switch w.fsm.State() {
	case StateRequestingZone:
		return w.enterRequestingZone(ctx)
	case StateProvisioning:
		return w.enterProvisioning(ctx)
	case StateRegistering:
		return w.enterRegistering(ctx)
	case StateRollingBack:
		return w.enterRollingBack(ctx)
	case StateStopping:
		return w.enterStopping(ctx)
	case StateRemoving:
		return w.enterRemoving(ctx)
	}
	// StatePending, StateRunning, StateComplete, StateError: no action.
	return nil
}

// applyEvent dispatches an incoming event to the appropriate handler.
// Handlers only update FSM state; all work is done in driveState.
func (w *MachineWorker) applyEvent(ctx context.Context, event MachineEvent) error {
	switch event.Type {
	case EventLifeChanged:
		return w.applyLifeChange(ctx, event.Life)
	case EventZoneAssigned:
		return w.applyZoneAssigned(ctx, event.Zone)
	case EventZoneRequestFailed:
		return w.applyZoneRequestFailed(ctx, event.ZoneError)
	default:
		w.config.Logger.Warningf(ctx, "machine %s received unknown event type %s", w.config.MachineID, event.Type)
		return nil
	}
}

// applyLifeChange handles lifecycle changes from the main loop.
//
// Life value semantics for the provisioner:
//   - Dying: the machine's workers in the machine agent manifold are still
//     running and handling shutdown. The provisioner must not provision or
//     de-provision; it waits for the machine to go dead.
//   - Dead: the machine's workers have completed their shutdown. The
//     provisioner can now de-provision the instance and mark the machine
//     for removal.
//   - The removal worker then looks for a dead machine with a dead or absent
//     instance to actually delete the machine record.
func (w *MachineWorker) applyLifeChange(ctx context.Context, newLife life.Value) error {
	w.config.Logger.Debugf(ctx, "machine %s applying life change to %s in state %s",
		w.config.MachineID, newLife, w.fsm.State())

	switch w.fsm.State() {
	case StatePending:
		// Cancel any pending retry timer since we're handling a life event.
		w.stopRetryTimer()

		if newLife == life.Dead {
			return w.transitionTo(ctx, StateRemoving)
		}
		if newLife == life.Dying {
			// Machine is shutting down; don't provision but wait for dead.
			w.config.Logger.Debugf(ctx, "machine %s is dying, waiting for dead", w.config.MachineID)
			return nil
		}
		// life.Alive - check if already provisioned.
		if w.instanceID != "" {
			w.config.Logger.Debugf(ctx, "machine %s already has instance %s, transitioning to running",
				w.config.MachineID, w.instanceID)
			return w.transitionTo(ctx, StateRunning)
		}
		// Not provisioned yet, start provisioning.
		return w.transitionTo(ctx, StateRequestingZone)

	case StateRequestingZone:
		if newLife == life.Dead {
			return w.transitionTo(ctx, StateRemoving)
		}
		if newLife == life.Dying {
			// Machine is shutting down; abort the zone request and go back to
			// pending. The stale zone response will be ignored when it arrives
			// since we'll no longer be in RequestingZone.
			w.config.Logger.Debugf(ctx, "machine %s is dying while requesting zone, returning to pending", w.config.MachineID)
			return w.transitionTo(ctx, StatePending)
		}
		// Still alive, waiting for zone response.

	case StateProvisioning, StateRegistering, StateRollingBack:
		// An active provider/state API call is in progress. Because the loop
		// skips the select while state transitions are chaining (prevState !=
		// state), this case is unreachable in normal operation. Life events are
		// buffered in EventsChan and processed once the worker reaches an idle
		// state (Running or Pending).

	case StateRunning:
		if newLife == life.Dead {
			if w.config.Machine.KeepInstance() || w.instanceID == "" {
				// keep-instance: preserve the cloud resource.
				// No instance: nothing to stop.
				w.config.Logger.Infof(ctx, "machine %s is dead, removing without stopping instance", w.config.MachineID)
				return w.transitionTo(ctx, StateRemoving)
			}
			return w.transitionTo(ctx, StateStopping)
		}
		// Dying: machine agent workers are still shutting down; wait for dead.

	case StateStopping, StateRemoving:
		// Already in terminal path, ignore life changes.

	case StateComplete, StateError:
		// Terminal state, nothing to do.
	}

	return nil
}

// applyZoneAssigned handles successful zone assignment from the AZ Coordinator.
// Transitions from RequestingZone → Provisioning; stale responses are dropped.
func (w *MachineWorker) applyZoneAssigned(ctx context.Context, zone string) error {
	if w.fsm.State() != StateRequestingZone {
		// Stale response (e.g. arrived after machine went dying → pending).
		w.config.Logger.Debugf(ctx, "machine %s ignoring stale zone assignment in state %s",
			w.config.MachineID, w.fsm.State())
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s assigned to zone %s", w.config.MachineID, zone)
	w.zoneName = zone
	return w.transitionTo(ctx, StateProvisioning)
}

// applyZoneRequestFailed handles a zone request failure from the AZ Coordinator.
// Increments the retry counter and transitions to Pending (retry) or Error
// (retries exhausted). Stale responses are dropped.
func (w *MachineWorker) applyZoneRequestFailed(ctx context.Context, err error) error {
	if w.fsm.State() != StateRequestingZone {
		// Stale response.
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

	if err := w.transitionTo(ctx, StatePending); err != nil {
		return err
	}
	w.scheduleRetry(ctx)
	return nil
}

// applyRetryTimer handles the retry timer firing.
// For StatePending: advances to StateRequestingZone so driveState sends the
// zone request. For other retry states (Stopping, Removing): no transition is
// needed because driveState will re-execute their action on the next iteration.
func (w *MachineWorker) applyRetryTimer(ctx context.Context) error {
	if w.fsm.State() == StatePending {
		return w.transitionTo(ctx, StateRequestingZone)
	}
	// StateStopping / StateRemoving: the timer just wakes up the loop;
	// driveState will retry the operation automatically.
	return nil
}

// ── State entry actions (one discrete operation per active state) ─────────────

// enterRequestingZone sends a zone request to the main loop.
// The state remains RequestingZone until a zone response event arrives.
func (w *MachineWorker) enterRequestingZone(ctx context.Context) error {
	msg := NewZoneRequestMessage(w.config.MachineID, w.config.DistributionGroup, w.config.Constraints)

	select {
	case w.config.MessageChan <- msg:
		w.config.Logger.Debugf(ctx, "machine %s sent zone request", w.config.MachineID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// enterProvisioning acquires the provider semaphore and calls StartInstance.
// On success: transitions to StateRegistering (instance created, not yet registered).
// On failure: transitions to StatePending (retry) or StateError (retries exhausted).
func (w *MachineWorker) enterProvisioning(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s starting instance in zone %s", w.config.MachineID, w.zoneName)

	if err := w.config.Semaphore.Acquire(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		return errors.Trace(err)
	}
	defer w.config.Semaphore.Release()

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

		w.zoneName = ""
		if err := w.transitionTo(ctx, StatePending); err != nil {
			return err
		}
		w.scheduleRetry(ctx)
		return nil
	}

	w.instanceID = result.InstanceID
	w.config.Logger.Infof(ctx, "machine %s started instance %s", w.config.MachineID, w.instanceID)
	return w.transitionTo(ctx, StateRegistering)
}

// enterRegistering calls SetInstanceInfo to register the instance with Juju state.
// On success: notifies the main loop and transitions to StateRunning.
// On failure: transitions to StateRollingBack to clean up the orphaned instance.
//
// NOTE: There is a window between instance creation (enterProvisioning) and
// database registration (here) where a failure could leave an orphaned instance.
// StateRollingBack handles the cleanup.
func (w *MachineWorker) enterRegistering(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s registering instance %s in zone %s",
		w.config.MachineID, w.instanceID, w.zoneName)

	err := w.config.InstanceInfoSetter.SetInstanceInfo(ctx, w.config.MachineID, w.instanceID, w.zoneName)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Context was cancelled, but we still need to roll back the instance
			// we created. Transition to StateRollingBack so enterRollingBack runs.
			w.config.Logger.Warningf(ctx, "machine %s context cancelled during SetInstanceInfo, will roll back instance %s",
				w.config.MachineID, w.instanceID)
		} else {
			w.config.Logger.Warningf(ctx, "machine %s SetInstanceInfo failed, rolling back: %v",
				w.config.MachineID, err)
		}
		w.lastError = err
		return w.transitionTo(ctx, StateRollingBack)
	}

	w.notifyProvisionSuccess(ctx)
	w.config.Logger.Infof(ctx, "machine %s provisioning complete, instance %s in zone %s",
		w.config.MachineID, w.instanceID, w.zoneName)
	return w.transitionTo(ctx, StateRunning)
}

// enterRollingBack stops the orphaned instance that was created by
// enterProvisioning but could not be registered by enterRegistering.
// After the cleanup attempt (whether or not StopInstances succeeds), the
// instance state is cleared and the worker transitions to StatePending (retry)
// or StateError (retries exhausted).
func (w *MachineWorker) enterRollingBack(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s rolling back: stopping orphaned instance %s",
		w.config.MachineID, w.instanceID)

	if err := w.config.Semaphore.Acquire(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		return errors.Trace(err)
	}
	defer w.config.Semaphore.Release()

	if stopErr := w.config.Broker.StopInstances(ctx, w.instanceID); stopErr != nil {
		// Log but do not abort: proceed with the retry regardless.
		// The orphaned instance may require manual cleanup.
		w.config.Logger.Errorf(ctx, "machine %s rollback StopInstances failed (instance %s may be orphaned): %v",
			w.config.MachineID, w.instanceID, stopErr)
	}

	w.notifyProvisionFailed(ctx, w.lastError)

	// Clear instance state before retrying.
	w.instanceID = ""
	w.zoneName = ""
	w.retryCount++

	w.config.Logger.Warningf(ctx, "machine %s provisioning failed after registration (attempt %d/%d): %v",
		w.config.MachineID, w.retryCount, w.config.MaxRetries+1, w.lastError)

	if w.retryCount > w.config.MaxRetries {
		w.config.Logger.Errorf(ctx, "machine %s provisioning retries exhausted after SetInstanceInfo failure", w.config.MachineID)
		if setErr := w.setProvisioningError(ctx, w.lastError); setErr != nil {
			w.config.Logger.Errorf(ctx, "failed to set provisioning error: %v", setErr)
		}
		return w.transitionTo(ctx, StateError)
	}

	if err := w.transitionTo(ctx, StatePending); err != nil {
		return err
	}
	w.scheduleRetry(ctx)
	return nil
}

// enterStopping acquires the provider semaphore and calls StopInstances.
// On success: transitions to StateRemoving.
// On failure: schedules a retry and stays in StateStopping. The retry timer
// wakes the loop which calls driveState again.
func (w *MachineWorker) enterStopping(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s stopping instance %s", w.config.MachineID, w.instanceID)

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
		w.lastError = err
		w.config.Logger.Warningf(ctx, "machine %s StopInstances failed, will retry: %v", w.config.MachineID, err)
		w.scheduleRetry(ctx)
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s instance %s stopped", w.config.MachineID, w.instanceID)
	return w.transitionTo(ctx, StateRemoving)
}

// enterRemoving calls MarkForRemoval on the machine record.
// On success: transitions to StateComplete (terminal).
// On failure: schedules a retry and stays in StateRemoving.
func (w *MachineWorker) enterRemoving(ctx context.Context) error {
	w.config.Logger.Infof(ctx, "machine %s removing machine record", w.config.MachineID)

	err := w.config.Machine.MarkForRemoval(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return w.catacomb.ErrDying()
		}
		w.lastError = err
		w.config.Logger.Warningf(ctx, "machine %s MarkForRemoval failed, will retry: %v", w.config.MachineID, err)
		w.scheduleRetry(ctx)
		return nil
	}

	w.config.Logger.Infof(ctx, "machine %s removed", w.config.MachineID)
	return w.transitionTo(ctx, StateComplete)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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

// notifyProvisionFailed sends a failure notification to the main loop.
func (w *MachineWorker) notifyProvisionFailed(ctx context.Context, err error) {
	msg := NewProvisionCompleteMessage(w.config.MachineID, "", w.zoneName, err)

	select {
	case w.config.MessageChan <- msg:
	case <-ctx.Done():
		w.config.Logger.Warningf(ctx, "machine %s failed to send provision failure notification", w.config.MachineID)
	}
}

// notifyProvisionSuccess sends a success notification to the main loop.
func (w *MachineWorker) notifyProvisionSuccess(ctx context.Context) {
	msg := NewProvisionCompleteMessage(w.config.MachineID, w.instanceID, w.zoneName, nil)

	select {
	case w.config.MessageChan <- msg:
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
