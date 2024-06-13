// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package keys provides the domain needed for configuring public keys on a
// model for a user.
//
// Public keys for a user are per model based and will not follow a user between
// models. Currently under the covers we do not model the public keys and their
// user (owner) as an old legacy implementation details of Juju 3.x.
package keymanager
