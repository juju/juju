// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bootstrap ensures that when the initial bootstrap process
// has started that we seed the following:
//
//  1. The macaroon config keypairs are generated.
//  2. The initial juju-metrics user is registered.
//  3. The agent binary is seeded into the objectstore.
//  4. The initial storage pools are created.
//  5. The controller application has been created, along with the supporting
//     machine and spaces.
//  6. The controller charm is seeded into the objectstore.
//  7. Seed any extra authorised keys into the controller model.
//  8. Finally, set a flag to indicate bootstrap has been completed
//
// The intention is that the worker is started as soon as possible during
// bootstrap and that it will uninstall itself once the bootstrap process
// has completed.
//
// If any of these steps fail, the bootstrap worker will restart. So it is
// important that seeding processes are idempotent.
package bootstrap
