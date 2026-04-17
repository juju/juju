// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// handleApplicationDomainDeployError is a first low pass effort to start
// handling some of the errors that will come out of the application domain
// during deploy. If a handler does not exist then the original error will be
// returned.
func handleApplicationDomainDeployError(err error) error {
	switch {
	// When a storage instance being attached to a new unit as part of deployment
	// has it's attachments change. This happens when a lot of change is
	// occurring between operations.
	case errors.Is(err, applicationerrors.StorageInstanceUnexpectedAttachments):
		return errors.New(
			"existing storage instance had it's attachments changed during deployment",
		)

	// When attaching an existing storage instance that is no longer alive.
	case errors.Is(err, storageerrors.StorageInstanceNotAlive):
		return errors.New(
			"storage instance is not alive during attach",
		).Add(coreerrors.NotValid)

	// When attaching an existing storage instance that no longer exists.
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.New(
			"storage instance not found during attach",
		).Add(coreerrors.NotFound)

	// When attaching an existing storage instance whose name is not defined in
	// the charm's storage definitions.
	case errors.Is(err, applicationerrors.StorageNameNotSupported):
		return errors.New(
			"storage instance to attach has a name that is not supported by the charm",
		).Add(coreerrors.NotValid)

	// When attaching an existing storage instance whose kind does
	// not match the charm storage definition (e.g. block vs filesystem).
	case errors.Is(err, applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition):
		return errors.New(
			"storage instance kind is not valid for charm storage definition during attach",
		).Add(coreerrors.NotValid)

	// When attaching an existing storage instance that is associated with a
	// different charm name than the application being deployed.
	case errors.Is(err, applicationerrors.StorageInstanceCharmNameMismatch):
		return errors.New(
			"storage instance charm name does not match application charm during attach",
		).Add(coreerrors.NotValid)

	// When attaching an existing storage instance whose size does not meet the
	// charm storage definition minimum size.
	case errors.Is(err, applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition):
		return errors.New(
			"storage instance size is not valid for charm storage definition during attach",
		).Add(coreerrors.NotValid)

	// When attaching an existing storage instance whose owning machine does not
	// match the unit's machine.
	case errors.Is(err, applicationerrors.StorageInstanceAttachMachineOwnerMismatch):
		return errors.New(
			"storage instance owning machine does not match unit machine during attach",
		).Add(coreerrors.NotValid)

	// When the supplied storage directive overrides violates the charm's
	// storage.
	case errors.HasType[applicationerrors.StorageCountLimitExceeded](err):
		limitErr, _ := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
		if limitErr.Requested < limitErr.Minimum {
			return errors.Errorf(
				"storage directive %q request count %d insufficient for the charm's minimum count of %d",
				limitErr.StorageName, limitErr.Requested, limitErr.Minimum,
			).Add(coreerrors.NotValid)
		} else if limitErr.Maximum != nil && limitErr.Requested > *limitErr.Maximum {
			return errors.Errorf(
				"storage directive %q request count %d exceeds the charm's maximum count of %d",
				limitErr.StorageName, limitErr.Requested, *limitErr.Maximum,
			).Add(coreerrors.NotValid)
		}
	// When the charm storage location violates a prohibited filesystem mount
	// point.
	case errors.HasType[applicationerrors.CharmStorageLocationProhibited](err):
		prohibitErr, _ := errors.AsType[applicationerrors.CharmStorageLocationProhibited](err)
		return errors.Errorf(
			"charm storage %q wants to use a prohibited location %q, must not be in %q",
			prohibitErr.CharmStorageName,
			prohibitErr.CharmStorageLocation,
			prohibitErr.ProhibitedLocation,
		).Add(coreerrors.NotValid)
	// When attaching storage to an application but the application name is not
	// a valid application name.
	case errors.Is(err, applicationerrors.ApplicationNameNotValid):
		return apiservererrors.ParamsErrorf(params.CodeNotValid,
			"application name is not valid",
		)
	}

	return err
}
