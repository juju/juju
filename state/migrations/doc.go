// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package migrations aims to create an intermediate state between state and the
// description package. One that aims to correctly model the dependencies
// required for migration.
//
// This package is in a state of transition, from the traditional way in the
// state package and the new way in this migrations package. It's likely to be
// a slow, considered move between the two and until all migrations are moved
// to here, there could be additional complexities. Once that is done, a new
// look to simplify any complexities which came up during the move is probably
// wise.
//
// Concept:
//
//	The key concept is to remove code (complexities) from the state package that
//	could be easily modelled somewhere else.
//
// Steps:
//   - Create a new Migration that can perform the execution.
//   - Create a source (src) and destination (dst) point of use interface
//   - Migrate from source to destination models.
package migrations
