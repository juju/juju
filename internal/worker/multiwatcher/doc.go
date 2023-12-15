// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package multiwatcher provides watchers that watch the entire model.
//
// The package is responsible for creating, feeding, and cleaning up after
// multiwatchers. The core worker gets an event stream from an
// AllWatcherBacking, and manages the multiwatcher Store.
//
// The behaviour of the multiwatchers is very much tied to the Store implementation.
// The store provides a mechanism to get changes over time.
package multiwatcher
