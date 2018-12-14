// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cache is responsible for keeping an in memory representation of the
// controller's models.
//
// The Controller is kept up to date with the database though a changes channel.
//
// Instances in the cache package also provide watchers. These watchers are
// checking for changes in the in-memory representation and can be used to avoid
// excess database reads.
package cache
