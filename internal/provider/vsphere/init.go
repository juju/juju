// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"
	"net/url"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/vsphere/internal/vsphereclient"
)

const (
	// providerType is the unique identifier that the vsphere provider gets
	// registered with.
	providerType = "vsphere"
)

func init() {
	dial := func(ctx context.Context, u *url.URL, dc string) (Client, error) {
		return vsphereclient.Dial(ctx, u, dc, logger)
	}
	environs.RegisterProvider(providerType, NewEnvironProvider(EnvironProviderConfig{
		Dial: dial,
	}))
}
