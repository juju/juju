// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/environs/context"
)

var NewAPIWithBacking = newAPIWithBacking

func SupportsSpaces(backing networkingcommon.NetworkBacking, ctx context.ProviderCallContext) error {
	api := &API{
		backing: backing,
		context: ctx,
	}
	return api.checkSupportsSpaces()
}
