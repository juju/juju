// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

type storageSolver struct {
	opFactory operation.Factory
	storage   *Attachments
	dying     bool
	life      map[names.StorageTag]params.Life
}

// NewSolver returns a new storage solver.
func NewSolver(opFactory operation.Factory, storage *Attachments) solver.Solver {
	return &storageSolver{
		opFactory: opFactory,
		storage:   storage,
		life:      make(map[names.StorageTag]params.Life),
	}
}

// NextOp is defined on the Solver interface.
func (s *storageSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	var changed []names.StorageTag
	for tag, storage := range remoteState.Storage {
		life, ok := s.life[tag]
		if !ok || life != storage.Life {
			s.life[tag] = storage.Life
			changed = append(changed, tag)
		}
	}
	for tag := range s.life {
		if _, ok := remoteState.Storage[tag]; !ok {
			changed = append(changed, tag)
			delete(s.life, tag)
		}
	}
	if len(changed) > 0 {
		return s.opFactory.NewUpdateStorage(changed)
	}
	if !opState.Installed && s.storage.Pending() == 0 {
		logger.Infof("initial storage attachments ready")
	}
	return s.nextOp(opState, remoteState)
}

func (s *storageSolver) nextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {
	if remoteState.Life == params.Dying {
		if !s.dying {
			if err := s.storage.SetDying(); err != nil {
				return nil, errors.Trace(err)
			}
			s.dying = true
		}
		for tag, snap := range remoteState.Storage {
			snap.Life = params.Dying
			remoteState.Storage[tag] = snap
		}
	}
	for tag, snap := range remoteState.Storage {
		op, err := s.nextHookOp(tag, snap)
		if errors.Cause(err) == solver.ErrNoOperation {
			continue
		}
		return op, err
	}
	if s.storage.Pending() > 0 {
		logger.Debugf("still pending %v", s.storage.pending)
		if !opState.Installed {
			return nil, solver.ErrWaiting
		}
	}
	return nil, solver.ErrNoOperation
}

func (s *storageSolver) nextHookOp(
	tag names.StorageTag, snap remotestate.StorageSnapshot,
) (operation.Operation, error) {
	logger.Debugf("next hook op for %v: %+v", tag, snap)
	storageAttachment, ok := s.storage.storageAttachments[tag]
	if !ok {
		return nil, solver.ErrNoOperation
	}
	switch snap.Life {
	case params.Alive:
		if storageAttachment.attached {
			// Storage attachments currently do not change
			// (apart from lifecycle) after being provisioned.
			// We don't process unprovisioned storage here,
			// so there's nothing to do.
			return nil, solver.ErrNoOperation
		}
	case params.Dying:
		if !storageAttachment.attached {
			// Nothing to do: attachment is dying, but
			// the storage-attached hook has not been
			// consumed.
			return nil, solver.ErrNoOperation
		}
	case params.Dead:
		// Storage must have been Dying to become Dead;
		// no further action is required.
		return nil, solver.ErrNoOperation
	}

	hookInfo := hook.Info{
		StorageId: tag.Id(),
	}
	if snap.Life == params.Alive {
		hookInfo.Kind = hooks.StorageAttached
	} else {
		hookInfo.Kind = hooks.StorageDetaching
	}
	context := &contextStorage{
		tag:      tag,
		kind:     storage.StorageKind(snap.Kind),
		location: snap.Location,
	}
	storageAttachment.ContextStorageAttachment = context
	s.storage.storageAttachments[tag] = storageAttachment

	logger.Debugf("queued hook: %v", hookInfo)
	return s.opFactory.NewRunHook(hookInfo)
}
