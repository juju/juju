// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offer

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
)

// ApplicationOfferArgs contains parameters used to create or update
// an application offer.
type ApplicationOfferArgs struct {
	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// OfferName is the name of the offer.
	OfferName string

	// OwnerName is the name of the owner of the offer.
	OwnerName user.Name
}

func (a ApplicationOfferArgs) Validate() error {
	if a.ApplicationName == "" {
		return errors.Errorf("application name cannot be empty").Add(coreerrors.NotValid)
	}
	if a.OwnerName.Name() == "" {
		return errors.Errorf("owner name cannot be empty").Add(coreerrors.NotValid)
	}
	if len(a.Endpoints) == 0 {
		return errors.Errorf("endpoints cannot be empty").Add(coreerrors.NotValid)
	}
	return nil
}
