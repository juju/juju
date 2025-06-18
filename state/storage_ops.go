// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
)

type addStorageForUnitOperation struct {
	sb                 *storageConfigBackend
	u                  *Unit
	storageName        string
	storageConstraints StorageConstraints

	// The list of storage tags after a the operation succeeds.
	tags []names.StorageTag
}

// Build implements ModelOperation.
func (op *addStorageForUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.u.refresh(); err != nil {
			return nil, errors.Annotatef(err, "adding %q storage to %s", op.storageName, op.u)
		}
	}

	tags, ops, err := op.sb.addStorageForUnitOps(op.u, op.storageName, op.storageConstraints)
	if err != nil {
		return nil, errors.Annotatef(err, "adding %q storage to %s", op.storageName, op.u)
	}

	op.tags = tags
	return ops, nil
}

// Done implements ModelOperation.
func (op *addStorageForUnitOperation) Done(err error) error { return err }
