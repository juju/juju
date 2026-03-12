// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

// NewAddressChangeWatcherForTest exposes the local address watcher factory for
// patching in external tests.
var NewAddressChangeWatcherForTest = &newAddressChangeWatcher
