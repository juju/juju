// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// StorageResolverOperations instances know how to make operations
// required by the resolver.
type StorageResolverOperations interface {
	NewUpdateStorage(tags []names.StorageTag) (operation.Operation, error)
	NewRunHook(hookInfo hook.Info) (operation.Operation, error)
}

type storageResolver struct {
	storage *Attachments
	dying   bool
	life    map[names.StorageTag]params.Life
}

// NewResolver returns a new storage resolver.
func NewResolver(storage *Attachments) resolver.Resolver {
	return &storageResolver{
		storage: storage,
		life:    make(map[names.StorageTag]params.Life),
	}
}

// NextOp is defined on the Resolver interface.
func (s *storageResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
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
		return opFactory.NewUpdateStorage(changed)
	}
	if !localState.Installed && s.storage.Pending() == 0 {
		logger.Infof("initial storage attachments ready")
	}
	return s.nextOp(localState, remoteState, opFactory)
}

func (s *storageResolver) nextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
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

	var runStorageHooks bool
	switch {
	case localState.Kind == operation.Continue:
		// There's nothing in progress.
		runStorageHooks = true
	case !localState.Installed && localState.Kind == operation.RunHook && localState.Step == operation.Queued:
		// The install operation completed, and there's an install
		// hook queued. Run storage-attached hooks first.
		runStorageHooks = true
	}

	if runStorageHooks {
		for tag, snap := range remoteState.Storage {
			op, err := s.nextHookOp(tag, snap, opFactory)
			if errors.Cause(err) == resolver.ErrNoOperation {
				continue
			}
			return op, err
		}
		if s.storage.Pending() > 0 {
			logger.Debugf("still pending %v", s.storage.pending)
			if !localState.Installed {
				return nil, resolver.ErrWaiting
			}
		}
	}
	return nil, resolver.ErrNoOperation
}

func (s *storageResolver) nextHookOp(
	tag names.StorageTag,
	snap remotestate.StorageSnapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	logger.Debugf("next hook op for %v: %+v", tag, snap)
	if !snap.Attached {
		return nil, resolver.ErrNoOperation
	}
	storageAttachment, ok := s.storage.storageAttachments[tag]
	if !ok {
		return nil, resolver.ErrNoOperation
	}
	switch snap.Life {
	case params.Alive:
		if storageAttachment.attached {
			// Storage attachments currently do not change
			// (apart from lifecycle) after being provisioned.
			// We don't process unprovisioned storage here,
			// so there's nothing to do.
			return nil, resolver.ErrNoOperation
		}
	case params.Dying:
		if !storageAttachment.attached {
			// Nothing to do: attachment is dying, but
			// the storage-attached hook has not been
			// consumed.
			return nil, resolver.ErrNoOperation
		}
	case params.Dead:
		// Storage must have been Dying to become Dead;
		// no further action is required.
		return nil, resolver.ErrNoOperation
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
	return opFactory.NewRunHook(hookInfo)
}
