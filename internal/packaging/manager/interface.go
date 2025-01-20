// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

// PackageManager is the interface which carries out various
// package-management related work.
//
// TODO (stickupid): Both the PackageManager and PackageCommander from the
// commands package should be merged. The layout of the packaging package is
// over-engineered. The commands should be placed directly into the package
// types themselves and then the managers could be a lot more simpler.
type PackageManager interface {
	// Install runs a *single* command that installs the given package(s).
	Install(packs ...string) error
}
