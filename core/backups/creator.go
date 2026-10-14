// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/juju/internal/errors"
)

const (
	// ErrBackupCreationWorkerDying is returned when the backup creation worker
	// is dying.
	ErrBackupCreationWorkerDying = errors.ConstError("backup creation worker is dying")
)

// BackupCreator creates a backup archive with the supplied notes. It honours
// context cancellation and returns only after generation and lease maintenance
// stop. On success, the caller must invoke cleanup after consuming the archive.
// On error, the implementation removes any partial or completed archive.

type BackupCreator interface {
	// Create creates an archive with the supplied notes. It honours context
	// cancellation and returns only after generation and lease maintenance stop.
	// On success, the caller must invoke cleanup after consuming the archive.
	// On error, the implementation removes any partial or completed archive.
	Create(ctx context.Context, notes string) (meta *Metadata, archive string, cleanup func(), err error)
}
