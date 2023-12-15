// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bootstrap ensures that when the initial bootstrap process
// has started that we seed the following:
//
//  1. The agent binary is seeded into the objectstore.
//  2. The controller application has been created, along with the supporting
//     machine and spaces.
//  3. The controller charm is seeded into the objectstore.
//
// The intention is that the worker is started as soon as possible during
// bootstrap and that it will uninstall itself once the bootstrap process
// has completed.
package bootstrap
