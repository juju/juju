// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package upgradeservices provides a controller worker that injects an
// UpgradeServices dependency into the manifold. Both this worker and the
// UpgradeServices implementation on offer MUST never take on any more
// dependencies then that of the controller database.
//
// By having a low dependency footprint it helps ensure that the controllers
// manifold never ends up in a dependency dead lock and that the upgrade
// database worker is able to run unlocking the rest of the manifold when
// complete.
package upgradeservices
