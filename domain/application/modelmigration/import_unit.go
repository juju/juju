// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service"
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

	var cloudContainer *application.CloudContainerParams
	if cc := unit.CloudContainer(); cc != nil {
		address, origin := makeAddress(cc.Address())

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

	return service.ImportUnitArg{
		UnitName:       unitName,
		PasswordHash:   passwordHash,
		CloudContainer: cloudContainer,
	}, nil
}
