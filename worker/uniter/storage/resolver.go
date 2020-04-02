// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
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
	storage   *Attachments
	dying     bool
	life      map[names.StorageTag]life.Value
	modelType model.ModelType
}

// NewResolver returns a new storage resolver.
func NewResolver(storage *Attachments, modelType model.ModelType) resolver.Resolver {
	return &storageResolver{
		storage:   storage,
		modelType: modelType,
		life:      make(map[names.StorageTag]life.Value),
	}
}

// NextOp is defined on the Resolver interface.
func (s *storageResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	if remoteState.Life == life.Dying {
		// The unit is dying, so destroy all of its storage.
		if !s.dying {
			if err := s.storage.SetDying(); err != nil {
				return nil, errors.Trace(err)
			}
			s.dying = true
		}
		for tag, snap := range remoteState.Storage {
			snap.Life = life.Dying
			remoteState.Storage[tag] = snap
		}
	}

	if err := s.maybeShortCircuitRemoval(remoteState.Storage); err != nil {
		return nil, errors.Trace(err)
	}

	// The decision making below with regard to when to run the storage hooks for
	// the first time after a charm is deployed is applicable only to IAAS models.
	// For IAAS models, we do not run install hook before any storage provisioned
	// with the deployment of the unit is available. The presumption is that IAAS
	// charms may need storage on which to install their workloads.
	//
	// For CAAS models, it's different because we need to create the pod before
	// any storage is provisioned. See in-line explanation below.
	//
	// TODO(juju3) - allow storage hooks to run after install for IAAS models
	// This will make IAAS and CAAS behaviour the same, and charms should be
	// resilient to resources such as storage not being ready at any given time.
	var runStorageHooks bool
	switch {
	case localState.Kind == operation.Continue:
		// There's nothing in progress.
		runStorageHooks = true
	case localState.Kind == operation.RunHook && localState.Step == operation.Queued:
		// There's a hook queued.
		switch s.modelType {
		case model.CAAS:
			// For CAAS models, we wait until after the start hook has run before
			// running storage-attached hooks since storage is provisioned only
			// after the pod has been created.
			//
			// NB we must be careful here. The initial hook sequence is
			// install->leader-elected->config-changed->started
			// This chain is activated by each preceding hook queuing the next.
			// If we allow a storage-attached hook to run before the start hook, we
			// will potentially overwrite the State to initiate the next hook.
			// So we need to get to at least started. This means that any charm logic
			// that needs storage cannot be in install or start and if in config-changed,
			// needs to be deferred until storage is available.
			runStorageHooks = localState.Started
		case model.IAAS:
			// For IAAS models, we run storage-attached hooks before install.
			runStorageHooks = !localState.Installed
		}
	}
	if !runStorageHooks {
		return nil, resolver.ErrNoOperation
	}

	// This message is only interesting for IAAS models.
	if s.modelType == model.IAAS && !localState.Installed && s.storage.Pending() == 0 {
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
		// For IAAS models, storage hooks are run before install.
		// If the install hook has not yet run and there's still
		// pending storage, we wait. We don't wait after the
		// install hook has run as we should not block other
		// hooks from running while new storage added post install
		// is being provisioned.
		if !localState.Installed && s.modelType == model.IAAS {
			return nil, resolver.ErrWaiting
		}
	}
	return nil, resolver.ErrNoOperation
}

// maybeShortCircuitRemoval removes any storage that is not alive,
// and has not had a storage-attached hook committed.
func (s *storageResolver) maybeShortCircuitRemoval(remote map[names.StorageTag]remotestate.StorageSnapshot) error {
	for tag, snap := range remote {
		attached, ok := s.storage.storageState.Attached(tag.Id())
		if (ok && attached) || snap.Life == life.Alive {
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

	if snap.Life == life.Dead {
		// Storage must have been Dying to become Dead;
		// no further action is required.
		return nil, resolver.ErrNoOperation
	}

	hookInfo := hook.Info{StorageId: tag.Id()}
	switch snap.Life {
	case life.Alive:
		attached, ok := s.storage.storageState.Attached(tag.Id())
		if ok && attached {
			// Once the storage is attached, we only care about
			// lifecycle State changes.
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
	case life.Dying:
		attached, ok := s.storage.storageState.Attached(tag.Id())
		if !ok || !attached {
			// Nothing to do: attachment is dying, but
			// the storage-attached hook has not been
			// issued.
			return nil, resolver.ErrNoOperation
		}
		// The storage is dying, but we haven't previously run the
		// "storage-detached" hook. Do so now.
		hookInfo.Kind = hooks.StorageDetaching
	}

	// Update the local State to reflect what we're about to report
	// to a hook.
	s.storage.storageAttachments[tag] = &contextStorage{
		tag:      tag,
		kind:     storage.StorageKind(snap.Kind),
		location: snap.Location,
	}

	return opFactory.NewRunHook(hookInfo)
}
