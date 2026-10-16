// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/network"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// NewControllerUnitAddressPatch prepares IP and DNS observations for recovery.
func NewControllerUnitAddressPatch(values []string) (domainrecovery.ControllerUnitAddressPatch, error) {
	var patch domainrecovery.ControllerUnitAddressPatch
	if len(values) == 0 {
		return patch, nil
	}
	deviceUUID, err := uuid.NewUUID()
	if err != nil {
		return patch, errors.Capture(err)
	}
	patch.DeviceUUID = deviceUUID.String()
	seen := make(map[string]bool)
	for _, value := range values {
		if value == "" {
			return domainrecovery.ControllerUnitAddressPatch{}, errors.New("empty controller unit address")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		addressUUID, err := uuid.NewUUID()
		if err != nil {
			return domainrecovery.ControllerUnitAddressPatch{}, errors.Capture(err)
		}
		patch.Addresses = append(patch.Addresses, domainrecovery.ControllerUnitAddress{
			UUID: addressUUID.String(), Value: value,
			Type: network.DeriveAddressType(value),
		})
	}
	return patch, nil
}
