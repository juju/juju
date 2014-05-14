// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// exported so we can get the deployer path from tests.
func GitDeployerDataPath(d Deployer) string {
	return d.(*gitDeployer).dataPath
}

// exported so we can get the deployer current git repo from tests.
func GitDeployerCurrent(d Deployer) *GitDir {
	return d.(*gitDeployer).current
}

func IsGitDeployer(d Deployer) bool {
	_, ok := d.(*gitDeployer)
	return ok
}

func IsManifestDeployer(d Deployer) bool {
	_, ok := d.(*manifestDeployer)
	return ok
}
