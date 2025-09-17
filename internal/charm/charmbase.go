// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// charmBase implements the Charm interface with commonality between
// a charm archive and directory.
type charmBase struct {
	meta       *Meta
	config     *ConfigSpec
	actions    *Actions
	lxdProfile *LXDProfile
	manifest   *Manifest
	version    string
	revision   int
}

// NewCharmBase creates a new charmBase with the given metadata, config,
// actions, lxdProfile, and manifest.
func NewCharmBase(meta *Meta, manifest *Manifest, config *ConfigSpec, actions *Actions, lxdProfile *LXDProfile) *charmBase {
	return &charmBase{
		meta:       meta,
		manifest:   manifest,
		config:     config,
		actions:    actions,
		lxdProfile: lxdProfile,
	}
}

// Revision returns the revision number for the charm
// expanded in dir.
func (c *charmBase) Revision() int {
	return c.revision
}

// Version returns the VCS version representing the version file from archive.
func (c *charmBase) Version() string {
	return c.version
}

// Meta returns the Meta representing the metadata.yaml file
// for the charm expanded in dir.
func (c *charmBase) Meta() *Meta {
	return c.meta
}

// Config returns the Config representing the config.yaml file
// for the charm expanded in dir.
func (c *charmBase) Config() *ConfigSpec {
	return c.config
}

// Actions returns the Actions representing the actions.yaml file
// for the charm expanded in dir.
func (c *charmBase) Actions() *Actions {
	return c.actions
}

// LXDProfile returns the LXDProfile representing the lxd-profile.yaml file
// for the charm expanded in dir.
func (c *charmBase) LXDProfile() *LXDProfile {
	return c.lxdProfile
}

// Manifest returns the Manifest representing the manifest.yaml file
// for the charm expanded in dir.
func (c *charmBase) Manifest() *Manifest {
	return c.manifest
}

// SetVersion changes the charm version. This affects
// the version reported by Version and the version of the
// charm created.
func (c *charmBase) SetVersion(version string) {
	c.version = version
}
