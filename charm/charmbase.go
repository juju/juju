// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// charmBase implements the Charm interface with commonality between
// a charm archive and directory.
type charmBase struct {
	meta       *Meta
	config     *Config
	actions    *Actions
	lxdProfile *LXDProfile
	manifest   *Manifest
	revision   int
	version    string
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
func (c *charmBase) Config() *Config {
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

// SetRevision changes the charm revision number. This affects
// the revision reported by Revision and the revision of the
// charm created.
// The revision file in the charm directory is not modified.
func (c *charmBase) SetRevision(revision int) {
	c.revision = revision
}
