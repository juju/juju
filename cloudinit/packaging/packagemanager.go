// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// this package provides a level of abstraction over the package managers
// available on different distributions
package packaging

import "fmt"

// PackageManager is a struct which returns system-specific commands for all
// the operations that may be required of a package management system
type PackageManager struct {
	// available options:
	// update				updates the local package list
	// upgrade				upgrades all packages
	// install packages...	installs the given packages
	// remove packages...	removes the given packages
	// purge packages...	removes the given packages along with all data
	// search package		searches for the given package
	// list-available		lists all packes available
	// list-installed		lists all installed packages
	// add-repository repo	adds the given repository
	// remove-repository	removes the given repository
	// cleanup				cleans up orhaned packages and the package cache
	cmds map[string]string
}

// Update returns the command that refreshes the local package list
func (p *PackageManager) Update() string {
	return p.cmds["update"]
}

// Upgrade returns the command that fetches all the available newer versions
// of the currently installed packages and installs them on the system
func (p *PackageManager) Upgrade() string {
	return p.cmds["upgrade"]
}

// Install returns the command that installs the given package(s)
func (p *PackageManager) Install(packs ...string) string {
	cmd := p.cmds["install"]

	for _, pack := range packs {
		cmd = cmd + pack + " "
	}

	return cmd[:len(cmd)-1]
}

// Remove returns the command that removes the given package(s)
// NOTE: yum: remove also has Purge()'s functionality
func (p *PackageManager) Remove(packs ...string) string {
	cmd := p.cmds["remove"]

	for _, pack := range packs {
		cmd = cmd + pack + " "
	}

	return cmd[:len(cmd)-1]
}

// Purge returns the command that removes the given package(s), along with all
// the auxiliary files associated to it(them)
func (p *PackageManager) Purge(packs ...string) string {
	cmd := p.cmds["purge"]

	for _, pack := range packs {
		cmd = cmd + pack + " "
	}

	return cmd[:len(cmd)-1]
}

// Search returns the command that determines whether the given package is
// available for installation from the currently configured repositories
func (p *PackageManager) Search(pack string) string {
	return fmt.Sprintf(p.cmds["search"], pack)
}

// ListAvailable returns the command which will list all packages available
// for installation from the currently configured repositories
// NOTE: includes already installed packages
func (p *PackageManager) ListAvailable() string {
	return p.cmds["list-available"]
}

// ListInstalled returns the command which will list all installed packages
// on the current system
func (p *PackageManager) ListInstalled() string {
	return p.cmds["list-installed"]
}

// ListRepositories returns the command to lists all repositories currently available
func (p *PackageManager) ListRepositories() string {
	return p.cmds["list-repositories"]
}

// AddRepository returns the command that adds a repository to the list of
// available repositories
func (p *PackageManager) AddRepository(repo string) string {
	return fmt.Sprintf(p.cmds["add-repository"], repo)
}

// RemoveRepository returns the command that removes a repository from the
// list of available repositories
func (p *PackageManager) RemoveRepository(repo string) string {
	return fmt.Sprintf(p.cmds["remove-repository"], repo)
}

// Cleanup returns the command that cleans up all orphaned packages, left-over
// configuration files and previously-cached packages
func (p *PackageManager) Cleanup() string {
	return p.cmds["cleanup"]
}
