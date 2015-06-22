// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/hook"
)

type acceptLeadership struct {
	DoesNotRequireMachineLock
}

// String is part of the Operation interface.
func (al *acceptLeadership) String() string {
	return "accept leadership"
}

// Prepare is part of the Operation interface.
func (al *acceptLeadership) Prepare(state State) (*State, error) {
	if err := al.checkState(state); err != nil {
		return nil, err
	}
	return nil, ErrSkipExecute
}

// Execute is part of the Operation interface.
func (al *acceptLeadership) Execute(state State) (*State, error) {
	return nil, errors.New("prepare always errors; Execute is never valid")
}

// Commit is part of the Operation interface.
func (al *acceptLeadership) Commit(state State) (*State, error) {
	if err := al.checkState(state); err != nil {
		return nil, err
	}
	if state.Leader {
		// Nothing needs to be done -- leader is only set when queueing a
		// leader-elected hook. Therefore, if leader is true, the appropriate
		// hook must be either queued or already run.
		return nil, nil
	}
	newState := stateChange{
		Kind: RunHook,
		Step: Queued,
		Hook: &hook.Info{Kind: hook.LeaderElected},
	}.apply(state)
	newState.Leader = true
	return newState, nil
}

func (al *acceptLeadership) checkState(state State) error {
	if state.Kind != Continue {
		// We'll need to queue up a hook, and we can't do that without
		// stomping on existing state.
		return ErrCannotAcceptLeadership
	}
	return nil
}

type resignLeadership struct {
	DoesNotRequireMachineLock
}

// String is part of the Operation interface.
func (rl *resignLeadership) String() string {
	return "resign leadership"
}

// Prepare is part of the Operation interface.
func (rl *resignLeadership) Prepare(state State) (*State, error) {
	if !state.Leader {
		// Nothing needs to be done -- state.Leader should only be set to
		// false when committing the leader-deposed hook. This code is not
		// helpful while Execute is a no-op, but it will become so.
		return nil, ErrSkipExecute
	}
	return nil, nil
}

// Execute is part of the Operation interface.
func (rl *resignLeadership) Execute(state State) (*State, error) {
	// TODO(fwereade): this hits a lot of interestingly intersecting problems.
	//
	// 1) we can't yet create a sufficiently dumbed-down hook context for a
	//    leader-deposed hook to run as specced. (This is the proximate issue,
	//    and is sufficient to prevent us from implementing this op right.)
	// 2) we want to write a state-file change, so this has to be an operation
	//    (or, at least, it has to be serialized with all other operations).
	//      * note that the change we write must *not* include the RunHook
	//        operation for leader-deposed -- we want to run this at high
	//        priority, in any possible state, and not to disturn what's
	//        there other than to note that we no longer think we're leader.
	// 3) the hook execution itself *might* not need to be serialized with
	//    other operations, which is moot until we consider that:
	// 4) we want to invoke this behaviour from elsewhere (ie when we don't
	//    have an api connection available), but:
	// 5) we can't get around the serialization requirement in (2).
	//
	// So. I *think* that the right approach is to implement a no-api uniter
	// variant, that we run *instead of* the normal uniter when the API is
	// unavailable, and replace with a real uniter when appropriate; this
	// implies that we need to take care not to allow the implementations to
	// diverge, but implementing them both as "uniters" is probably the best
	// way to encourage logic-sharing and prevent that problem.
	//
	// In the short term, though, we can just run leader-deposed as soon as we
	// can build the right environment. Not sure whether this particular type
	// will still be justified, or whether it'll just be a plain old RunHook --
	// I *think* it will stay, because the state-writing behaviour will stay
	// very different (ie just write `.Leader = false` and don't step on pre-
	// queued hooks).
	logger.Warningf("we should run a leader-deposed hook here, but we can't yet")
	return nil, nil
}

// Commit is part of the Operation interface.
func (rl *resignLeadership) Commit(state State) (*State, error) {
	state.Leader = false
	return &state, nil
}
