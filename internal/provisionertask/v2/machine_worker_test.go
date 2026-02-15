// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/juju/errors"
	tc "github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// Test suite runner.
func TestMachineWorkerSuite(t *testing.T) {
	tc.Run(t, &MachineWorkerSuite{})
}

// MachineWorkerSuite contains unit tests for MachineWorker FSM.
type MachineWorkerSuite struct{}

// fakeMachine implements MachineAPI for testing.
type fakeMachine struct {
	mu           sync.Mutex
	id           string
	life         life.Value
	instanceID   string
	keepInstance bool

	// Call tracking.
	ensureDeadCalled      bool
	markForRemovalCalled  bool
	lastStatus            status.Status
	lastStatusMsg         string
	lastInstanceStatus    status.Status
	lastInstanceStatusMsg string

	// Error injection.
	ensureDeadErr        error
	markForRemovalErr    error
	setStatusErr         error
	setInstanceStatusErr error
}

func newFakeMachine(id string) *fakeMachine {
	return &fakeMachine{
		id:   id,
		life: life.Alive,
	}
}

func (m *fakeMachine) ID() string {
	return m.id
}

func (m *fakeMachine) Life() life.Value {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.life
}

func (m *fakeMachine) InstanceID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instanceID
}

func (m *fakeMachine) KeepInstance() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keepInstance
}

func (m *fakeMachine) EnsureDead(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureDeadCalled = true
	if m.ensureDeadErr != nil {
		return m.ensureDeadErr
	}
	m.life = life.Dead
	return nil
}

func (m *fakeMachine) SetStatus(ctx context.Context, st status.Status, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setStatusErr != nil {
		return m.setStatusErr
	}
	m.lastStatus = st
	m.lastStatusMsg = message
	return nil
}

func (m *fakeMachine) SetInstanceStatus(ctx context.Context, st status.Status, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setInstanceStatusErr != nil {
		return m.setInstanceStatusErr
	}
	m.lastInstanceStatus = st
	m.lastInstanceStatusMsg = message
	return nil
}

func (m *fakeMachine) MarkForRemoval(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markForRemovalErr != nil {
		return m.markForRemovalErr
	}
	m.markForRemovalCalled = true
	return nil
}

func (m *fakeMachine) setLife(l life.Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.life = l
}

func (m *fakeMachine) setInstanceID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instanceID = id
}

func (m *fakeMachine) setKeepInstance(keep bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keepInstance = keep
}

func (m *fakeMachine) wasMarkForRemovalCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.markForRemovalCalled
}

func (m *fakeMachine) getLastInstanceStatus() status.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastInstanceStatus
}

// fakeBroker implements InstanceBroker for testing.
type fakeBroker struct {
	mu sync.Mutex

	// Call tracking.
	startInstanceCalls []StartInstanceParams
	stopInstancesCalls [][]string

	// Results and error injection.
	startInstanceResult StartInstanceResult
	startInstanceErr    error
	stopInstancesErr    error

	// Gates for blocking operations.
	startInstanceGate chan struct{}
	stopInstancesGate chan struct{}
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{}
}

func (b *fakeBroker) StartInstance(ctx context.Context, params StartInstanceParams) (StartInstanceResult, error) {
	// Wait on gate if set (for testing blocking behavior).
	if b.startInstanceGate != nil {
		select {
		case <-b.startInstanceGate:
		case <-ctx.Done():
			return StartInstanceResult{}, ctx.Err()
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.startInstanceCalls = append(b.startInstanceCalls, params)
	if b.startInstanceErr != nil {
		err := b.startInstanceErr
		// Reset error for next call unless it's a permanent error.
		return StartInstanceResult{}, err
	}
	return b.startInstanceResult, nil
}

func (b *fakeBroker) StopInstances(ctx context.Context, instanceIDs ...string) error {
	// Wait on gate if set.
	if b.stopInstancesGate != nil {
		select {
		case <-b.stopInstancesGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.stopInstancesCalls = append(b.stopInstancesCalls, instanceIDs)
	return b.stopInstancesErr
}

func (b *fakeBroker) setStartInstanceResult(result StartInstanceResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startInstanceResult = result
}

func (b *fakeBroker) setStartInstanceErr(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startInstanceErr = err
}

func (b *fakeBroker) getStartInstanceCallCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.startInstanceCalls)
}

func (b *fakeBroker) getStopInstancesCallCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.stopInstancesCalls)
}

func (b *fakeBroker) getStopInstancesCalls() [][]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopInstancesCalls
}

// fakeInstanceInfoSetter implements InstanceInfoSetter for testing.
type fakeInstanceInfoSetter struct {
	mu sync.Mutex

	// Call tracking.
	calls []setInstanceInfoCall

	// Error injection.
	err error

	// Gate for blocking.
	gate chan struct{}
}

type setInstanceInfoCall struct {
	MachineID  string
	InstanceID string
	ZoneName   string
}

func newFakeInstanceInfoSetter() *fakeInstanceInfoSetter {
	return &fakeInstanceInfoSetter{}
}

func (s *fakeInstanceInfoSetter) SetInstanceInfo(ctx context.Context, machineID, instanceID, zoneName string) error {
	// Wait on gate if set.
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, setInstanceInfoCall{
		MachineID:  machineID,
		InstanceID: instanceID,
		ZoneName:   zoneName,
	})
	return s.err
}

func (s *fakeInstanceInfoSetter) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *fakeInstanceInfoSetter) getCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// fakeSemaphore implements ProviderSemaphore for testing.
type fakeSemaphore struct {
	mu       sync.Mutex
	acquired int
	released int
}

func newFakeSemaphore() *fakeSemaphore {
	return &fakeSemaphore{}
}

func (s *fakeSemaphore) Acquire(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acquired++
	return nil
}

func (s *fakeSemaphore) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released++
}

func (s *fakeSemaphore) getAcquired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquired
}

func (s *fakeSemaphore) getReleased() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.released
}

// workerTestHarness provides a test setup for MachineWorker tests.
type workerTestHarness struct {
	c           *tc.C
	machine     *fakeMachine
	broker      *fakeBroker
	infoSetter  *fakeInstanceInfoSetter
	semaphore   *fakeSemaphore
	eventsChan  chan MachineEvent
	messageChan chan WorkerMessage
	distGroup   []string
	constraints []string
	maxRetries  int
	retryDelay  time.Duration
}

func newWorkerTestHarness(c *tc.C, machineID string) *workerTestHarness {
	return &workerTestHarness{
		c:           c,
		machine:     newFakeMachine(machineID),
		broker:      newFakeBroker(),
		infoSetter:  newFakeInstanceInfoSetter(),
		semaphore:   newFakeSemaphore(),
		eventsChan:  make(chan MachineEvent, 10),
		messageChan: make(chan WorkerMessage, 10),
		maxRetries:  2,
		retryDelay:  1 * time.Millisecond, // Very short for tests.
	}
}

func (h *workerTestHarness) config() MachineWorkerConfig {
	return MachineWorkerConfig{
		MachineID:          h.machine.ID(),
		Machine:            h.machine,
		Broker:             h.broker,
		InstanceInfoSetter: h.infoSetter,
		Semaphore:          h.semaphore,
		Logger:             loggertesting.WrapCheckLog(h.c),
		EventsChan:         h.eventsChan,
		MessageChan:        h.messageChan,
		DistributionGroup:  h.distGroup,
		Constraints:        h.constraints,
		MaxRetries:         h.maxRetries,
		RetryDelay:         h.retryDelay,
	}
}

func (h *workerTestHarness) sendEvent(event MachineEvent) {
	h.eventsChan <- event
}

func (h *workerTestHarness) receiveMessage() WorkerMessage {
	return <-h.messageChan
}

func (h *workerTestHarness) tryReceiveMessage() (WorkerMessage, bool) {
	select {
	case msg := <-h.messageChan:
		return msg, true
	default:
		return WorkerMessage{}, false
	}
}

// Tests.

func (s *MachineWorkerSuite) TestHappyPathProvisioning(c *tc.C) {
	// Test: Pending -> RequestingZone -> Provisioning -> Running.
	h := newWorkerTestHarness(c, "0")
	h.broker.setStartInstanceResult(StartInstanceResult{
		InstanceID: "i-0",
		ZoneName:   "zone-a",
	})

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, worker)

	// Worker starts in Pending state.
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// Send life=Alive to trigger provisioning.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})

	// Wait for zone request.
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	c.Assert(msg.MachineID, tc.Equals, "0")

	// Worker should be in RequestingZone state.
	c.Assert(worker.fsm.State(), tc.Equals, StateRequestingZone)

	// Send zone assignment.
	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Wait for provision complete notification.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	payload := msg.Payload.(ProvisionResultPayload)
	c.Assert(payload.Error, tc.IsNil)
	c.Assert(payload.InstanceID, tc.Equals, "i-0")
	c.Assert(payload.ZoneName, tc.Equals, "zone-a")

	// Worker should be in Running state.
	c.Assert(worker.fsm.State(), tc.Equals, StateRunning)

	// No StopInstances or Remove called.
	c.Assert(h.broker.getStopInstancesCallCount(), tc.Equals, 0)
	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsFalse)
}

func (s *MachineWorkerSuite) TestPendingLifeDeadRemovesWithoutProvisioning(c *tc.C) {
	// Test: Pending + life=Dead -> Removing -> Done.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// Send life=Dead.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Worker should exit cleanly after reaching Done.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// Remove called, no StartInstance.
	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsTrue)
	c.Assert(h.broker.getStartInstanceCallCount(), tc.Equals, 0)
}

func (s *MachineWorkerSuite) TestPendingLifeDyingDoesNotProvision(c *tc.C) {
	// Test: Pending + life=Dying -> stays Pending, no provisioning.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, worker)

	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// Send life=Dying.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dying})

	// Give time for event processing.
	time.Sleep(20 * time.Millisecond)

	// Worker should still be in Pending state - no provisioning started.
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)
	c.Assert(h.broker.getStartInstanceCallCount(), tc.Equals, 0)

	// No messages should have been sent.
	_, hasMsg := h.tryReceiveMessage()
	c.Assert(hasMsg, tc.IsFalse)
}

func (s *MachineWorkerSuite) TestRequestingZoneThenLifeDeadRemoves(c *tc.C) {
	// Test: Drive worker to RequestingZone, then life=Dead.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Trigger zone request.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})

	// Wait for zone request.
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	c.Assert(worker.fsm.State(), tc.Equals, StateRequestingZone)

	// Send life=Dead before zone assignment.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Worker should exit cleanly.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// Remove called, no StartInstance.
	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsTrue)
	c.Assert(h.broker.getStartInstanceCallCount(), tc.Equals, 0)
}

func (s *MachineWorkerSuite) TestRequestingZoneThenLifeDyingReturnsToPending(c *tc.C) {
	// Test: Drive worker to RequestingZone, then life=Dying -> back to Pending.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Trigger zone request.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})

	// Wait for zone request.
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	c.Assert(worker.fsm.State(), tc.Equals, StateRequestingZone)

	// Send life=Dying.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dying})

	// Give time for event processing.
	time.Sleep(20 * time.Millisecond)

	// Worker should be back in Pending state.
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)
	c.Assert(h.broker.getStartInstanceCallCount(), tc.Equals, 0)

	// Now send life=Dead to trigger removal.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Worker should exit cleanly.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsTrue)
}

func (s *MachineWorkerSuite) TestMachineDiesDuringProvisioning(c *tc.C) {
	// Test: Provisioning blocked, life=Dead event queued, processed after unblock.
	h := newWorkerTestHarness(c, "0")

	// Set up blocking gate for StartInstance.
	h.broker.startInstanceGate = make(chan struct{})
	h.broker.setStartInstanceResult(StartInstanceResult{
		InstanceID: "i-0",
		ZoneName:   "zone-a",
	})

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Trigger provisioning.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)

	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Worker is now blocked in StartInstance.
	// Queue a life=Dead event.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Unblock StartInstance.
	close(h.broker.startInstanceGate)

	// Wait for provision complete.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)

	// Now the queued life=Dead should be processed.
	// Worker should stop the instance and remove.

	// Worker should exit cleanly.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// Instance was created and then stopped.
	c.Assert(h.broker.getStartInstanceCallCount(), tc.Equals, 1)
	c.Assert(h.broker.getStopInstancesCallCount(), tc.Equals, 1)
	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsTrue)
}

func (s *MachineWorkerSuite) TestRollbackOnSetInstanceInfoFailure(c *tc.C) {
	// Test: StartInstance succeeds, SetInstanceInfo fails -> rollback StopInstances.
	h := newWorkerTestHarness(c, "0")
	h.maxRetries = 1
	h.broker.setStartInstanceResult(StartInstanceResult{
		InstanceID: "i-0",
		ZoneName:   "zone-a",
	})

	// First SetInstanceInfo fails, second succeeds.
	h.infoSetter.err = errors.New("registration failed")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Trigger provisioning.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)

	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Wait for provision failure notification.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	payload := msg.Payload.(ProvisionResultPayload)
	c.Assert(payload.Error, tc.Not(tc.IsNil))

	// Verify rollback: StopInstances was called for the created instance.
	c.Assert(h.broker.getStopInstancesCallCount(), tc.Equals, 1)
	stopCalls := h.broker.getStopInstancesCalls()
	c.Assert(stopCalls[0], tc.DeepEquals, []string{"i-0"})

	// Now fix the error - retry timer will fire automatically.
	h.infoSetter.setErr(nil)

	// Worker will automatically retry via timer and request zone again.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	c.Assert(worker.fsm.State(), tc.Equals, StateRequestingZone)

	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Wait for success.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	payload = msg.Payload.(ProvisionResultPayload)
	c.Assert(payload.Error, tc.IsNil)

	c.Assert(worker.fsm.State(), tc.Equals, StateRunning)
}

func (s *MachineWorkerSuite) TestRetryExhaustionSetsProvisioningError(c *tc.C) {
	// Test: StartInstance fails repeatedly until retries exhausted.
	h := newWorkerTestHarness(c, "0")
	h.maxRetries = 1 // Only 2 attempts total (initial + 1 retry).
	h.broker.setStartInstanceErr(errors.New("provider error"))

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// First attempt.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// First failure - worker schedules a retry.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	payload := msg.Payload.(ProvisionResultPayload)
	c.Assert(payload.Error, tc.Not(tc.IsNil))

	// Retry timer fires automatically and requests zone again.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)
	c.Assert(worker.fsm.State(), tc.Equals, StateRequestingZone)
	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Second failure -> retries exhausted.
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	payload = msg.Payload.(ProvisionResultPayload)
	c.Assert(payload.Error, tc.Not(tc.IsNil))

	// Worker should exit cleanly after setting ProvisioningError.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// ProvisioningError status should be set.
	c.Assert(h.machine.getLastInstanceStatus(), tc.Equals, status.ProvisioningError)

	// No more messages should be emitted.
	_, hasMore := h.tryReceiveMessage()
	c.Assert(hasMore, tc.IsFalse)
}

func (s *MachineWorkerSuite) TestKeepInstanceOnDeadWhileRunning(c *tc.C) {
	// Test: Dead while Running + keep-instance=true -> no StopInstances.
	h := newWorkerTestHarness(c, "0")
	h.machine.setKeepInstance(true)
	h.machine.setInstanceID("i-0") // Already has instance.

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Machine already has instance - life.Alive goes directly to Running.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})

	// Give time for state transition.
	time.Sleep(20 * time.Millisecond)
	c.Assert(worker.fsm.State(), tc.Equals, StateRunning)

	// Send life=Dead with keep-instance=true.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Worker should exit cleanly.
	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// No StopInstances called!
	c.Assert(h.broker.getStopInstancesCallCount(), tc.Equals, 0)

	// But Remove should still be called.
	c.Assert(h.machine.wasMarkForRemovalCalled(), tc.IsTrue)
}

func (s *MachineWorkerSuite) TestOrphanedDeadMachineCleanup(c *tc.C) {
	// Test: Dead machine with no instance -> remove without StopInstances.
	h := newWorkerTestHarness(c, "0")
	// No instance ID set.

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker)

	// Provision to Running (creates instance).
	h.broker.setStartInstanceResult(StartInstanceResult{
		InstanceID: "i-0",
		ZoneName:   "zone-a",
	})
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})
	msg := h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageRequestZone)

	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})
	msg = h.receiveMessage()
	c.Assert(msg.Type, tc.Equals, MessageProvisionComplete)
	c.Assert(worker.fsm.State(), tc.Equals, StateRunning)

	// Create a new worker for an orphaned machine (no instance).
	h2 := newWorkerTestHarness(c, "1")
	worker2, err := NewMachineWorker(h2.config())
	c.Assert(err, tc.IsNil)
	defer workertest.DirtyKill(c, worker2)

	// Send life=Dead directly.
	h2.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	// Worker should exit cleanly.
	err = workertest.CheckKilled(c, worker2)
	c.Assert(err, tc.IsNil)

	// No StopInstances called (no instance to stop).
	c.Assert(h2.broker.getStopInstancesCallCount(), tc.Equals, 0)

	// Remove was called.
	c.Assert(h2.machine.wasMarkForRemovalCalled(), tc.IsTrue)
}

func (s *MachineWorkerSuite) TestSemaphoreAcquireRelease(c *tc.C) {
	// Test: Semaphore is properly acquired and released during provisioning.
	h := newWorkerTestHarness(c, "0")
	h.broker.setStartInstanceResult(StartInstanceResult{
		InstanceID: "i-0",
		ZoneName:   "zone-a",
	})

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, worker)

	// Provision.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Alive})
	h.receiveMessage() // Zone request.

	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})
	h.receiveMessage() // Provision complete.

	c.Assert(worker.fsm.State(), tc.Equals, StateRunning)

	// Semaphore should have been acquired and released once for provisioning.
	c.Assert(h.semaphore.getAcquired(), tc.Equals, 1)
	c.Assert(h.semaphore.getReleased(), tc.Equals, 1)

	// Now stop the instance.
	h.sendEvent(MachineEvent{Type: EventLifeChanged, Life: life.Dead})

	err = workertest.CheckKilled(c, worker)
	c.Assert(err, tc.IsNil)

	// Semaphore should have been acquired and released again for stopping.
	c.Assert(h.semaphore.getAcquired(), tc.Equals, 2)
	c.Assert(h.semaphore.getReleased(), tc.Equals, 2)
}

func (s *MachineWorkerSuite) TestStaleZoneAssignedIgnored(c *tc.C) {
	// Test: Zone assigned event in wrong state is ignored.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, worker)

	// Worker is in Pending state.
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// Send zone assigned without requesting zone first (stale event).
	h.sendEvent(MachineEvent{Type: EventZoneAssigned, Zone: "zone-a"})

	// Worker should still be in Pending state (event ignored).
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// No messages should have been sent.
	_, hasMsg := h.tryReceiveMessage()
	c.Assert(hasMsg, tc.IsFalse)
}

func (s *MachineWorkerSuite) TestStaleZoneRequestFailedIgnored(c *tc.C) {
	// Test: Zone request failed event in wrong state is ignored.
	h := newWorkerTestHarness(c, "0")

	worker, err := NewMachineWorker(h.config())
	c.Assert(err, tc.IsNil)
	defer workertest.CleanKill(c, worker)

	// Worker is in Pending state.
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)

	// Send zone request failed without requesting zone first (stale event).
	h.sendEvent(MachineEvent{Type: EventZoneRequestFailed, ZoneError: errors.New("stale error")})

	// Worker should still be in Pending state (event ignored).
	c.Assert(worker.fsm.State(), tc.Equals, StatePending)
}

func (s *MachineWorkerSuite) TestConfigValidation(c *tc.C) {
	// Test: Config validation catches invalid configurations.
	h := newWorkerTestHarness(c, "0")

	// Missing MachineID.
	cfg := h.config()
	cfg.MachineID = ""
	_, err := NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*empty MachineID.*")

	// Missing Machine.
	cfg = h.config()
	cfg.Machine = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil Machine.*")

	// Missing Broker.
	cfg = h.config()
	cfg.Broker = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil Broker.*")

	// Missing InstanceInfoSetter.
	cfg = h.config()
	cfg.InstanceInfoSetter = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil InstanceInfoSetter.*")

	// Missing Semaphore.
	cfg = h.config()
	cfg.Semaphore = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil Semaphore.*")

	// Missing Logger.
	cfg = h.config()
	cfg.Logger = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil Logger.*")

	// Missing EventsChan.
	cfg = h.config()
	cfg.EventsChan = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil EventsChan.*")

	// Missing MessageChan.
	cfg = h.config()
	cfg.MessageChan = nil
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*nil MessageChan.*")

	// Negative MaxRetries.
	cfg = h.config()
	cfg.MaxRetries = -1
	_, err = NewMachineWorker(cfg)
	c.Assert(err, tc.ErrorMatches, ".*negative MaxRetries.*")
}
