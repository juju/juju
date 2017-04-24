// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"net/url"
	"time"

	"github.com/juju/mutex"
	"github.com/juju/utils/clock"
	"golang.org/x/net/context"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
)

const (
	providerType = "vsphere"
)

func init() {
	dial := func(ctx context.Context, u *url.URL, dc string) (Client, error) {
		return vsphereclient.Dial(ctx, u, dc, logger)
	}
	environs.RegisterProvider(providerType, NewEnvironProvider(EnvironProviderConfig{
		Dial:        dial,
		OVACacheDir: "/var/cache/juju/vsphere/ova",
		OVACacheLocker: NewMutexCacheLocker(mutex.Spec{
			Name:  "juju-vsphere",
			Clock: clock.WallClock,
			Delay: 5 * time.Second,
		}),
	}))
}
