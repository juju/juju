// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package lxdnames provides names for the lxd provider.
package lxdnames

// NOTE: this package exists to get around circular imports from cloud and
// provider/lxd.

// DefaultCloud is the name of the default lxd cloud, which corresponds to
// the local lxd daemon.
const DefaultCloud = "localhost"

// DefaultRegion is the name of the only "region" we support in lxd currently,
// which corresponds to the local lxd daemon.
const DefaultRegion = "localhost"

// ProviderType defines the provider/cloud type for lxd.
const ProviderType = "lxd"
