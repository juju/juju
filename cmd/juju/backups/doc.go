// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backups provides the juju backups command.
// Backup of juju's state is a critical feature, not only for juju users
// but for use inside juju itself.

// Backing up juju state involves exporting every database (the controller
// database and one per model) and copying the controller's data directory
// files, including the local object store (charms, resources and agent
// binaries). All the files are bundled up into a single archive. The
// archive carries a metadata.json manifest recording the source agent
// version, controller UUID and HA node count.

// The controller creates the backup file in a gzipped tar file with the
// following structure, then streams it to the local user's disk:
// juju-backup/
//     metadata.json             - the provenance record for the archive.
//     manifest.json             - the content index: every component's
//                                 path, kind, size and SHA-256 hash.
//     root.tar                  - the bundle of data-directory files.
//     dump/controller.yaml      - the controller database export.
//     dump/models/<uuid>.yaml   - one export per model database.

// Each database is exported in its own transaction, so the archive is not
// a single point-in-time snapshot of the whole controller; create backups
// during a quiet window (see the create-backup help).

// To recover an archive, bootstrap a fresh replacement controller with:
//   juju bootstrap <cloud> <name> --recovery <archive> --recovery-sha256 <hex>

package backups
