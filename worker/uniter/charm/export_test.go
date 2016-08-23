// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// exported so we can get the deployer path from tests.
func GitDeployerDataPath(d Deployer) string {
	return d.(*gitDeployer).dataPath
}

func IsManifestDeployer(d Deployer) bool {
	_, ok := d.(*manifestDeployer)
	return ok
}
