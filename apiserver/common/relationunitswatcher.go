// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// RelationUnitsWatcher represents a relation.RelationUnitsWatcher at the
// apiserver level (different type for changes).
type RelationUnitsWatcher = watcher.Watcher[params.RelationUnitsChange]
