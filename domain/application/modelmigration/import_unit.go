// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/errors"
)

func (i *importOperation) importUnit(ctx context.Context, unit description.Unit) (service.ImportUnitArg, error) {
	unitName, err := coreunit.NewName(unit.Name())
	if err != nil {
		return service.ImportUnitArg{}, err
	}

	var passwordHash *string
	if hash := unit.PasswordHash(); hash != "" {
		passwordHash = ptr(hash)
	}

	var principal coreunit.Name
	if unit.Principal() != "" {
		principal, err = coreunit.NewName(unit.Principal())
		if err != nil {
			return service.ImportUnitArg{}, errors.Capture(err)
		}
	}

	return service.ImportUnitArg{
		UnitName:     unitName,
		PasswordHash: passwordHash,
		Principal:    principal,
	}, nil
}

func (i *importOperation) importCAASUnit(ctx context.Context, unit description.Unit) (service.ImportUnitArg, error) {
	unitArgs, err := i.importUnit(ctx, unit)
	if err != nil {
		return service.ImportUnitArg{}, errors.Capture(err)
	}

	var cloudContainer *application.CloudContainerParams
	if cc := unit.CloudContainer(); cc != nil {
		address, origin := i.makeAddress(cc.Address())

		cloudContainer = &application.CloudContainerParams{
			Address:       address,
			AddressOrigin: origin,
		}
		if cc.ProviderId() != "" {
			cloudContainer.ProviderID = cc.ProviderId()
		}
		if len(cc.Ports()) > 0 {
			cloudContainer.Ports = ptr(cc.Ports())
		}
	}
	unitArgs.CloudContainer = cloudContainer

	return unitArgs, nil
}

func (i *importOperation) importIAASUnit(ctx context.Context, unit description.Unit) (service.ImportUnitArg, error) {
	unitArgs, err := i.importUnit(ctx, unit)
	if err != nil {
		return service.ImportUnitArg{}, errors.Capture(err)
	}

	unitArgs.Machine = machine.Name(unit.Machine())
	return unitArgs, nil
}
