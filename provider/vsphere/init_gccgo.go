// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build gccgo

// This file exists so that this package will remain importable under
// GCCGo. In particular, see provider/all/all.go. All other files in
// this package do not build under GCCGo (see lp:1440940).

package vsphere

const (
	providerType = "vsphere"
)
