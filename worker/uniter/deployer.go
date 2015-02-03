// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/charm"
)

// deployerProxy exists because we're not yet comfortable that we can safely
// drop support for charm.gitDeployer. If we can, then the uniter doesn't
// need a deployer reference at all: and we can drop the Fix method, and even
// the Notify* methods on the Deployer interface, and simply hand the
// deployer we create over to the operationFactory at creation and forget
// about it. But.
//
// We will never be *completely* certain that gitDeployer can be dropped,
// because it's not done as an upgrade step (because we can't replace the
// deployer while conflicted, and upgrades are not gated on no-conflicts);
// and so long as there's a reasonable possibility that someone *might* have
// been running a pre-1.19.1 environment, and have either upgraded directly
// in a conflict state *or* have upgraded stepwise without fixing a conflict
// state, we should keep this complexity.
//
// In practice, that possibility is growing ever more remote, but we're not
// ready to pull the trigger yet.
type deployerProxy struct {
	charm.Deployer
}

// Fix replaces a git-based charm deployer with a manifest-based one, if
// necessary. It should not be called unless the existing charm deployment
// is known to be in a stable state.
func (d *deployerProxy) Fix() error {
	if err := charm.FixDeployer(&d.Deployer); err != nil {
		return errors.Annotatef(err, "cannot convert git deployment to manifest deployment")
	}
	return nil
}

// NotifyRevert is part of the charm.Deployer interface.
func (d *deployerProxy) NotifyRevert() error {
	if err := d.Deployer.NotifyRevert(); err != nil {
		return err
	}
	// Now we've reverted, we can guarantee that the deployer is in a sane state;
	// it's a great time to replace the git deployer (if we're still using it).
	return d.Fix()
}
