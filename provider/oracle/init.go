// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import "github.com/juju/juju/environs"

func init() {
	environs.RegisterProvider(providerType, &environProvider{})
}
