// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/juju/core/logger"

// NewDeployer returns a manifest deployer. It is a var so that it can be
// patched for uniter tests.
var NewDeployer = newDeployer

// NewDeployerFunc returns a func used to create a deployer.
type NewDeployerFunc func(charmPath, dataPath string, bundles BundleReader, logger logger.Logger) (Deployer, error)

func newDeployer(charmPath, dataPath string, bundles BundleReader, logger logger.Logger) (Deployer, error) {
	return NewManifestDeployer(charmPath, dataPath, bundles, logger), nil
}
