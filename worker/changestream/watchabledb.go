// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
)

type watchableDB struct {
	database.TxnRunner
	changestream.EventSource
}

func newWatchableDB(db database.TxnRunner, events changestream.EventSource) *watchableDB {
	return &watchableDB{
		TxnRunner:   db,
		EventSource: events,
	}
}
