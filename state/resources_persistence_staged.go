// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// StagedResource represents resource info that has been added to the
// "staging" area of the underlying data store. It remains unavailable
// until finalized, at which point it moves out of the staging area and
// replaces the current active resource info.
type StagedResource struct {
	base   ResourcePersistenceBase
	id     string
	stored storedResource
}

func (staged StagedResource) stage() error {
	// TODO(ericsnow) Ensure that the service is still there?

	buildTxn := func(attempt int) ([]txn.Op, error) {
		var ops []txn.Op
		switch attempt {
		case 0:
			ops = newInsertStagedResourceOps(staged.stored)
		case 1:
			ops = newEnsureStagedResourceSameOps(staged.stored)
		default:
			return nil, errors.NewAlreadyExists(nil, "already staged")
		}

		return ops, nil
	}
	if err := staged.base.Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Unstage ensures that the resource is removed
// from the staging area. If it isn't in the staging area
// then this is a noop.
func (staged StagedResource) Unstage() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// The op has no assert so we should not get here.
			return nil, errors.New("unstaging the resource failed")
		}

		ops := newRemoveStagedResourceOps(staged.id)
		return ops, nil
	}
	if err := staged.base.Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Activate makes the staged resource the active resource.
func (staged StagedResource) Activate() error {
	// TODO(ericsnow) Ensure that the service is still there?

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// This is an "upsert".
		var ops []txn.Op
		switch attempt {
		case 0:
			ops = newInsertResourceOps(staged.stored)
		case 1:
			ops = newUpdateResourceOps(staged.stored)
		default:
			return nil, errors.New("setting the resource failed")
		}
		// No matter what, we always remove any staging.
		ops = append(ops, newRemoveStagedResourceOps(staged.id)...)

		// If we are changing the bytes for a resource, we increment the
		// CharmModifiedVersion on the service, since resources are integral to
		// the high level "version" of the charm.
		if staged.stored.PendingID == "" {
			hasNewBytes, err := staged.hasNewBytes()
			if err != nil {
				logger.Errorf("can't read existing resource during activate: %v", errors.Details(err))
				return nil, errors.Trace(err)
			}
			if hasNewBytes {
				incOps := staged.base.IncCharmModifiedVersionOps(staged.stored.ServiceID)
				ops = append(ops, incOps...)
			}
		}
		logger.Debugf("activate ops: %#v", ops)
		return ops, nil
	}
	if err := staged.base.Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (staged StagedResource) hasNewBytes() (bool, error) {
	var current resourceDoc
	err := staged.base.One(resourcesC, staged.stored.ID, &current)
	switch {
	case errors.IsNotFound(err):
		// if there's no current resource stored, then any non-zero bytes will
		// be new.
		return !staged.stored.Fingerprint.IsZero(), nil
	case err != nil:
		return false, errors.Annotate(err, "couldn't read existing resource")
	default:
		diff := !bytes.Equal(staged.stored.Fingerprint.Bytes(), current.Fingerprint)
		return diff, nil
	}
}
