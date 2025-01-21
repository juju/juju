// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging

// PackageManagerName describes a package manager.
type PackageManagerName string

// The list of supported package managers.
const (
	AptPackageManager  PackageManagerName = "apt"
	SnapPackageManager PackageManagerName = "snap"
)
