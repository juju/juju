// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// exported so we can get the deployer path from tests.
func (d *Deployer) Path() string {
	return d.path
}

// exported so we can get the deployer current git repo from tests.
func (d *Deployer) Current() *GitDir {
	return d.current
}
