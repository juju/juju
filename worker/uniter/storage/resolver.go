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

	if remoteState.Life == params.Dying {
		// The unit is dying, so destroy all of its storage.
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

	if err := s.maybeShortCircuitRemoval(remoteState.Storage); err != nil {
		return nil, errors.Trace(err)
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
	if !runStorageHooks {
		return nil, resolver.ErrNoOperation
	}

	if !localState.Installed && s.storage.Pending() == 0 {
		logger.Infof("initial storage attachments ready")
	}

	for tag, snap := range remoteState.Storage {
		op, err := s.nextHookOp(tag, snap, opFactory)
		if errors.Cause(err) == resolver.ErrNoOperation {
			continue
		}
		return op, err
	}
	if s.storage.Pending() > 0 {
		logger.Debugf("still pending %v", s.storage.pending.SortedValues())
		if !localState.Installed {
			// We only wait for pending storage before
			// the install hook runs; we should not block
			// other hooks from running while storage is
			// being provisioned.
			return nil, resolver.ErrWaiting
		}
	}
	return nil, resolver.ErrNoOperation
}

// maybeShortCircuitRemoval removes any storage that is not alive,
// and has not had a storage-attached hook committed.
func (s *storageResolver) maybeShortCircuitRemoval(remote map[names.StorageTag]remotestate.StorageSnapshot) error {
	for tag, snap := range remote {
		local, ok := s.storage.storageAttachments[tag]
		if (ok && local.attached) || snap.Life == params.Alive {
			continue
		}
		if err := s.storage.removeStorageAttachment(tag); err != nil {
			return errors.Trace(err)
		}
		delete(remote, tag)
	}
	return nil
}

func (s *storageResolver) nextHookOp(
	tag names.StorageTag,
	snap remotestate.StorageSnapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	logger.Debugf("next hook op for %v: %+v", tag, snap)

	if snap.Life == params.Dead {
		// Storage must have been Dying to become Dead;
		// no further action is required.
		return nil, resolver.ErrNoOperation
	}

	hookInfo := hook.Info{StorageId: tag.Id()}
	switch snap.Life {
	case params.Alive:
		storageAttachment, ok := s.storage.storageAttachments[tag]
		if ok && storageAttachment.attached {
			// Once the storage is attached, we only care about
			// lifecycle state changes.
			return nil, resolver.ErrNoOperation
		}
		// The storage-attached hook has not been committed, so add the
		// storage to the pending set.
		s.storage.pending.Add(tag)
		if !snap.Attached {
			// The storage attachment has not been provisioned yet,
			// so just ignore it for now. We'll be notified again
			// when it has been provisioned.
			return nil, resolver.ErrNoOperation
		}
		// The storage is alive, but we haven't previously run the
		// "storage-attached" hook. Do so now.
		hookInfo.Kind = hooks.StorageAttached
	case params.Dying:
		storageAttachment, ok := s.storage.storageAttachments[tag]
		if !ok || !storageAttachment.attached {
			// Nothing to do: attachment is dying, but
			// the storage-attached hook has not been
			// issued.
			return nil, resolver.ErrNoOperation
		}
		// The storage is dying, but we haven't previously run the
		// "storage-detached" hook. Do so now.
		hookInfo.Kind = hooks.StorageDetaching
	}

	// Update the local state to reflect what we're about to report
	// to a hook.
	stateFile, err := readStateFile(s.storage.storageStateDir, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	s.storage.storageAttachments[tag] = storageAttachment{
		stateFile, &contextStorage{
			tag:      tag,
			kind:     storage.StorageKind(snap.Kind),
			location: snap.Location,
		},
	}

	return opFactory.NewRunHook(hookInfo)
}
