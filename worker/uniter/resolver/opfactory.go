package resolver

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
)

var logger = loggo.GetLogger("juju.worker.uniter.resolver")

type OpFactory struct {
	operation.Factory

	LocalState  LocalState
	RemoteState remotestate.Snapshot
}

func (s *OpFactory) NewRunHook(info hook.Info) (operation.Operation, error) {
	op, err := s.Factory.NewRunHook(info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapHookOp(op, info), nil
}

func (s *OpFactory) NewSkipHook(info hook.Info) (operation.Operation, error) {
	op, err := s.Factory.NewSkipHook(info)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapHookOp(op, info), nil
}

func (s *OpFactory) NewUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *OpFactory) NewRevertUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewRevertUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *OpFactory) NewResolvedUpgrade(charmURL *charm.URL) (operation.Operation, error) {
	op, err := s.Factory.NewResolvedUpgrade(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.wrapUpgradeOp(op, charmURL), nil
}

func (s *OpFactory) wrapUpgradeOp(op operation.Operation, charmURL *charm.URL) operation.Operation {
	return onCommitWrapper{op, func() {
		s.LocalState.CharmURL = charmURL
		s.LocalState.Upgraded = true
	}}
}

func (s *OpFactory) wrapHookOp(op operation.Operation, info hook.Info) operation.Operation {
	switch info.Kind {
	case hooks.ConfigChanged:
		v := s.RemoteState.ConfigVersion
		op = onCommitWrapper{op, func() { s.LocalState.ConfigVersion = v }}
	case hooks.LeaderSettingsChanged:
		v := s.RemoteState.LeaderSettingsVersion
		op = onCommitWrapper{op, func() { s.LocalState.LeaderSettingsVersion = v }}
	}
	return op
}

type onCommitWrapper struct {
	operation.Operation
	f func()
}

func (op onCommitWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	op.f()
	return st, nil
}
