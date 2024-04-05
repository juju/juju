// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coresecrets "github.com/juju/juju/core/secrets"
)

// SecretOwnerFromTag converts the tag string to a secret owner.
func SecretOwnerFromTag(ownerTag string) (coresecrets.Owner, error) {
	owner, err := names.ParseTag(ownerTag)
	if err != nil {
		return coresecrets.Owner{}, errors.Trace(err)
	}
	switch owner.Kind() {
	case names.ApplicationTagKind:
		return coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: owner.Id()}, nil
	case names.UnitTagKind:
		return coresecrets.Owner{Kind: coresecrets.UnitOwner, ID: owner.Id()}, nil
	case names.ModelTagKind:
		return coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: owner.Id()}, nil
	}
	return coresecrets.Owner{}, errors.NotValidf("owner tag kind %q", owner.Kind())
}

// OwnerTagFromSecretOwner converts the secret owner to a tag.
func OwnerTagFromSecretOwner(owner coresecrets.Owner) (names.Tag, error) {
	switch owner.Kind {
	case coresecrets.UnitOwner:
		return names.NewUnitTag(owner.ID), nil
	case coresecrets.ApplicationOwner:
		return names.NewApplicationTag(owner.ID), nil
	case coresecrets.ModelOwner:
		return names.NewModelTag(owner.ID), nil
	}
	return nil, errors.NotValidf("owner kind %q", owner.Kind)
}
