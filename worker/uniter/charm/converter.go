// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// NewDeployer returns a manifest deployer. It is a var so that it can be
// patched for uniter tests.
var NewDeployer = newDeployer

func newDeployer(charmPath, dataPath string, bundles BundleReader, logger Logger) (Deployer, error) {
	return NewManifestDeployer(charmPath, dataPath, bundles, logger), nil
}
