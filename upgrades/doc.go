// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The upgrades package provides infrastructure to upgrade previous Juju
// deployments to the current Juju version. The upgrade is performed on
// a per node basis, across all of the running Juju machines.
//
// Important exported APIs include:
//   PerformUpgrade, which is invoked on each node by the machine agent with:
//     fromVersion - the Juju version from which the upgrade is occurring
//     target      - the type of Juju node being upgraded
//     context     - provides API access to Juju state servers
//
package upgrades
