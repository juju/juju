// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/nustate/persistence/transaction"
)

type MachinePortRanges interface {
	transaction.Element

	// Getters
	MachineID() string
	ForUnit(string) *UnitPortRanges

	//Setters
	SetPortRanges(map[string]*UnitPortRanges)
	Remove()
}

type UnitPortRanges struct {
	OpenPortsByEndpoint map[string][]network.PortRange
}

func (upr *UnitPortRanges) ForEndpoint(endpointName string) []network.PortRange {
	if upr.OpenPortsByEndpoint == nil {
		return nil
	}
	return upr.OpenPortsByEndpoint[endpointName]
}

func (upr *UnitPortRanges) ByEndpoint() map[string][]network.PortRange {
	// TODO: copy map
	return upr.OpenPortsByEndpoint
}
