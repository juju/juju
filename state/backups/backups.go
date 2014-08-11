// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// backups contains all the stand-alone backup-related functionality for
// juju state.
package backups

import (
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"
)

var logger = loggo.GetLogger("juju.state.backups")

// Backups is an abstraction around all juju backup-related functionality.
type Backups interface {
}

type backups struct {
	storage filestorage.FileStorage
}

// NewBackups returns a new Backups value using the provided DB info and
// file storage.
func NewBackups(stor filestorage.FileStorage) Backups {
	b := backups{
		storage: stor,
	}
	return &b
}
