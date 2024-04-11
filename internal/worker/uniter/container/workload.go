// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"context"
	"strconv"
	"sync"

	"github.com/juju/charm/v13/hooks"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

// Logger defines the logging methods that the package uses.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
}

// WorkloadEventType is used to distinguish between each event type triggered
// by the workload.
type WorkloadEventType int

const (
	// ReadyEvent is triggered when the container/pebble starts up.
	ReadyEvent WorkloadEventType = iota
	CustomNoticeEvent
	ChangeUpdatedEvent
)

// WorkloadEvent contains information about the event type and data associated with
// the event.
type WorkloadEvent struct {
	Type         WorkloadEventType
	WorkloadName string
	NoticeID     string
	NoticeType   string
	NoticeKey    string
}

// WorkloadEventCallback is the type used to callback when an event has been processed.
type WorkloadEventCallback func(err error)

// WorkloadEvents is an interface providing a means of storing and retrieving
// arguments for running workload hooks.
type WorkloadEvents interface {
	// AddWorkloadEvent adds the given command arguments and response function
	// and returns a unique identifier.
	AddWorkloadEvent(evt WorkloadEvent, cb WorkloadEventCallback) string

	// GetWorkloadEvent returns the command arguments and response function
	// with the specified ID, as registered in AddWorkloadEvent.
	GetWorkloadEvent(id string) (WorkloadEvent, WorkloadEventCallback, error)

	// RemoveWorkloadEvent removes the command arguments and response function
	// associated with the specified ID.
	RemoveWorkloadEvent(id string)

	// Events returns all the currently queued events pending processing.
	// Useful for debugging/testing.
	Events() []WorkloadEvent

	// EventIDs returns all the ids for the events currently queued.
	EventIDs() []string
}

type workloadEvents struct {
	mu      sync.Mutex
	nextID  int
	pending map[string]workloadEventItem
}

type workloadEventItem struct {
	WorkloadEvent
	cb WorkloadEventCallback
}

// NewWorkloadEvents returns a new workload event queue.
func NewWorkloadEvents() WorkloadEvents {
	return &workloadEvents{pending: make(map[string]workloadEventItem)}
}

func (c *workloadEvents) AddWorkloadEvent(evt WorkloadEvent, cb WorkloadEventCallback) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := strconv.Itoa(c.nextID)
	c.nextID++
	c.pending[id] = workloadEventItem{evt, cb}
	return id
}

func (c *workloadEvents) RemoveWorkloadEvent(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *workloadEvents) GetWorkloadEvent(id string) (WorkloadEvent, WorkloadEventCallback, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.pending[id]
	if !ok {
		return WorkloadEvent{}, nil, errors.NotFoundf("workload event %s", id)
	}
	return item.WorkloadEvent, item.cb, nil
}

func (c *workloadEvents) Events() []WorkloadEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	events := make([]WorkloadEvent, 0, len(c.pending))
	for _, v := range c.pending {
		events = append(events, v.WorkloadEvent)
	}
	return events
}

func (c *workloadEvents) EventIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.pending))
	for id := range c.pending {
		ids = append(ids, id)
	}
	return ids
}

type workloadHookResolver struct {
	logger         Logger
	events         WorkloadEvents
	eventCompleted func(id string)
}

// NewWorkloadHookResolver returns a new resolver with determines which workload related operation
// should be run based on local and remote uniter states.
func NewWorkloadHookResolver(logger Logger, events WorkloadEvents, eventCompleted func(string)) resolver.Resolver {
	return &workloadHookResolver{
		logger:         logger,
		events:         events,
		eventCompleted: eventCompleted,
	}
}

// NextOp implements the resolver.Resolver interface.
func (r *workloadHookResolver) NextOp(
	ctx context.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	noOp := func() (operation.Operation, error) {
		if localState.Kind == operation.RunHook &&
			localState.Hook != nil && localState.Hook.Kind.IsWorkload() {
			// If we are resuming from an unexpected state, skip hook.
			return opFactory.NewSkipHook(*localState.Hook)
		}
		return nil, resolver.ErrNoOperation
	}

	switch localState.Kind {
	case operation.RunHook:
		if localState.Step != operation.Pending ||
			localState.Hook == nil || !localState.Hook.Kind.IsWorkload() {
			break
		}
		fallthrough

	case operation.Continue:
		for _, id := range remoteState.WorkloadEvents {
			evt, cb, err := r.events.GetWorkloadEvent(id)
			if errors.Is(err, errors.NotFound) {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}

			done := func(err error) {
				cb(err)
				r.events.RemoveWorkloadEvent(id)
				if r.eventCompleted != nil {
					r.eventCompleted(id)
				}
			}

			var op operation.Operation
			switch evt.Type {
			case CustomNoticeEvent:
				op, err = opFactory.NewRunHook(hook.Info{
					Kind:         hooks.PebbleCustomNotice,
					WorkloadName: evt.WorkloadName,
					NoticeID:     evt.NoticeID,
					NoticeType:   evt.NoticeType,
					NoticeKey:    evt.NoticeKey,
				})
			case ChangeUpdatedEvent:
				op, err = opFactory.NewRunHook(hook.Info{
					Kind:         hooks.PebbleChangeUpdated,
					WorkloadName: evt.WorkloadName,
					NoticeID:     evt.NoticeID,
					NoticeType:   evt.NoticeType,
					NoticeKey:    evt.NoticeKey,
				})
			case ReadyEvent:
				op, err = opFactory.NewRunHook(hook.Info{
					Kind:         hooks.PebbleReady,
					WorkloadName: evt.WorkloadName,
				})
			default:
				return nil, errors.NotValidf("workload event type %v", evt.Type)
			}
			if err != nil {
				done(err)
				return nil, errors.Trace(err)
			}
			return &errorWrappedOp{
				op,
				done,
			}, nil
		}
		return noOp()
	}

	return noOp()
}

// errorWrappedOp calls the handler function when any Prepare, Execute or Commit fail.
// On success handler will be called once with a nil error.
type errorWrappedOp struct {
	operation.Operation
	handler func(error)
}

// Prepare is part of the Operation interface.
func (op *errorWrappedOp) Prepare(ctx context.Context, state operation.State) (*operation.State, error) {
	newState, err := op.Operation.Prepare(ctx, state)
	if err != nil && err != operation.ErrSkipExecute {
		op.handler(err)
	}
	return newState, err
}

// Execute is part of the Operation interface.
func (op *errorWrappedOp) Execute(ctx context.Context, state operation.State) (*operation.State, error) {
	newState, err := op.Operation.Execute(ctx, state)
	if err != nil {
		op.handler(err)
	}
	return newState, err
}

// Commit preserves the recorded hook, and returns a neutral state.
// Commit is part of the Operation interface.
func (op *errorWrappedOp) Commit(ctx context.Context, state operation.State) (*operation.State, error) {
	newState, err := op.Operation.Commit(ctx, state)
	op.handler(err)
	return newState, err
}

// WrappedOperation is part of the WrappedOperation interface.
func (op *errorWrappedOp) WrappedOperation() operation.Operation {
	return op.Operation
}
