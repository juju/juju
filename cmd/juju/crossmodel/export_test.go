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
	cmd := &offerCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewShowEndpointsCommandForTest(api ShowAPI) cmd.Command {
	cmd := &showCommand{api: api}
	return envcmd.Wrap(cmd)
}
