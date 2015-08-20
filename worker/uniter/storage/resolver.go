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
	"github.com/juju/juju/worker/uniter/resolver"
)

// StorageResolverOperations instances know how to make operations
// required by the resolver.
type StorageResolverOperations interface {
	NewUpdateStorage(tags []names.StorageTag) (operation.Operation, error)
	NewRunHook(hookInfo hook.Info) (operation.Operation, error)
}

type storageResolver struct {
	opFactory StorageResolverOperations
	storage   *Attachments
	dying     bool
	life      map[names.StorageTag]params.Life
}

// NewResolver returns a new storage resolver.
func NewResolver(opFactory StorageResolverOperations, storage *Attachments) resolver.Resolver {
	return &storageResolver{
		opFactory: opFactory,
		storage:   storage,
		life:      make(map[names.StorageTag]params.Life),
	}
}

// NextOp is defined on the Resolver interface.
func (s *storageResolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	var changed []names.StorageTag
	for _, storage := range remoteState.Storage {
		life, ok := s.life[storage.Tag]
		if !ok || life != storage.Life {
			s.life[storage.Tag] = storage.Life
			changed = append(changed, storage.Tag)
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

func (s *storageResolver) nextOp(
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
	for _, snap := range remoteState.Storage {
		op, err := s.nextHookOp(snap)
		if errors.Cause(err) == resolver.ErrNoOperation {
			continue
		}
		return op, err
	}
	if s.storage.Pending() > 0 {
		logger.Debugf("still pending %v", s.storage.pending)
		if !opState.Installed {
			return nil, resolver.ErrWaiting
		}
	}
	return nil, resolver.ErrNoOperation
}

func (s *storageResolver) nextHookOp(snap remotestate.StorageSnapshot) (operation.Operation, error) {
	logger.Debugf("next hook op for %+v", snap)
	storageAttachment, ok := s.storage.storageAttachments[snap.Tag]
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
