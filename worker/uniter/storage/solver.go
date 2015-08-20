// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
	"github.com/juju/utils/set"
)

type storageSolver struct {
	opFactory operation.Factory
	storage   *Attachments

	knownStorage set.Tags
}

// NewSolver returns a new storage solver.
func NewSolver(opFactory operation.Factory, storage *Attachments) solver.Solver {
	return &storageSolver{
		opFactory:    opFactory,
		storage:      storage,
		knownStorage: set.NewTags(),
	}
}

// NextOp is defined on the Solver interface.
func (s *storageSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	allTags := set.NewTags()
	allTags = allTags.Union(s.knownStorage)
	for tag, _ := range remoteState.Storage {
		allTags.Add(tag)
	}

	if len(s.knownStorage) != len(allTags) {
		logger.Debugf("setting up storage for %v", allTags)
		s.knownStorage = s.knownStorage.Union(allTags)
		storage := make([]names.StorageTag, len(allTags))
		for i, tag := range allTags.SortedValues() {
			storage[i] = tag.(names.StorageTag)
		}
		return s.opFactory.NewUpdateStorage(storage)
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
	for _, snap := range remoteState.Storage {
		op, err := s.nextHookOp(snap)
		if err == solver.ErrNoOperation {
			continue
		}
		if err != nil {
			return nil, err
		}
		return op, nil
	}
	if s.storage.Pending() > 0 {
		logger.Debugf("still pending %v", s.storage.pending)
		if !opState.Installed {
			return nil, solver.ErrWaiting
		}
	}

	return nil, solver.ErrNoOperation
}

func (s *storageSolver) nextHookOp(snap remotestate.StorageSnapshot) (operation.Operation, error) {
	logger.Debugf("next hook op for %+v", snap)
	storageAttachment, ok := s.storage.storageAttachments[snap.Tag]
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
		// Storage must been Dying to become Dead;
		// no further action is required.
		return nil, solver.ErrNoOperation
	}

	hookInfo := hook.Info{
		StorageId: snap.Tag.Id(),
	}
	if snap.Life == params.Alive {
		hookInfo.Kind = hooks.StorageAttached
	} else {
		hookInfo.Kind = hooks.StorageDetaching
	}
	context := &contextStorage{
		tag:      snap.Tag,
		kind:     storage.StorageKind(snap.Kind),
		location: snap.Location,
	}
	storageAttachment.ContextStorageAttachment = context
	s.storage.storageAttachments[snap.Tag] = storageAttachment

	logger.Debugf("queued hook: %v", hookInfo)
	return s.opFactory.NewRunHook(hookInfo)
}
