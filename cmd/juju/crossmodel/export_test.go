// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

var (
	Max          = max
	DescAt       = descAt
	BreakLines   = breakLines
	ColumnWidth  = columnWidth
	BreakOneWord = breakOneWord
)

func NewOfferCommandForTest(api OfferAPI) cmd.Command {
	aCmd := &offerCommand{newAPIFunc: func() (OfferAPI, error) {
		return api, nil
	}}
	return envcmd.Wrap(aCmd)
}

func NewShowEndpointsCommandForTest(api ShowAPI) cmd.Command {
	aCmd := &showCommand{newAPIFunc: func() (ShowAPI, error) {
		return api, nil
	}}
	return envcmd.Wrap(aCmd)
}

func NewListEndpointsCommandForTest(api ListAPI) cmd.Command {
	aCmd := &listCommand{newAPIFunc: func() (ListAPI, error) {
		return api, nil
	}}
	return envcmd.Wrap(aCmd)
}

func NewFindEndpointsCommandForTest(api FindAPI) cmd.Command {
	aCmd := &findCommand{newAPIFunc: func() (FindAPI, error) {
		return api, nil
	}}
	return envcmd.Wrap(aCmd)
}
