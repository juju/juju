// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "database/sql"

type State struct {
	db *sql.DB
}

func NewState(db *sql.DB) *State {
	return &State{
		db: db,
	}
}

func (s *State) DB() *sql.DB {
	return s.db
}
