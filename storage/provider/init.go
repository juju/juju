// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

func init() {
	RegisterProvider(LoopProviderType, &loopProvider{logAndExec})

	// TODO(axw) provide a function for registering common storage providers.
}
