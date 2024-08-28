// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backups provides the juju backups command.
// Backup of juju's state is a critical feature, not only for juju users
// but for use inside juju itself.

// Backing up juju state involves dumping the state database and copying all
// files that are critical to juju's operation. All the files are bundled up
// into an archive file. Effectively the archive represents a snapshot of juju state.

// The controller creates the backup file in a gzipped tar file with the following structure, then streams it to the local user's disk:
// juju-backup/
//     metadata.json - the backup metadata for the archive.
//     root.tar      - the bundle of state-related files (exluding mongo).
//     dump/         - all the files dumped from the DB (using mongodump).

// At present we do not include any sort of manifest/index file in the
// archive.

// For more information, see:
//   - state/backups/db/dump.go     - how the DB is dumped;
//   - state/backups/files/files.go - which files are included in root.tar.

// The current backup process doesn't block state changes, meaning the database dump
// might be slightly outdated by the time all state-related files are gathered,
// though the risk is minimal.

// In terms of the restore process, please see "[juju-restore tool]".
// [juju-restore tool]: https://github.com/juju/juju-restore

package backups
