// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

// Package schema provides a way to manage the schema of a database.
// By providing a list of patches, the schema can be brought up to date.
// A schema table is used to keep track of the current version of the schema.
// Rerunning the same patches is idempotent, so it is safe to run the same
// patches multiple times.
// All patches are run within a transaction, so if any patch fails, the
// transaction is rolled back and the error is returned.
//
// Each patch has a hash associated with it, which is used to detect if the
// patch has changed. If the patch has changed, the schema is considered
// corrupted and the process is aborted. Once a release of Juju has been made,
// the patches should never be changed. This is to ensure that schema upgrades
// are built on top of a stable base of patches.
// The hashes are computed by summing up the previous patch hashes, forming a
// chain of hashes. This ensures that if any patch changes, all subsequent
// patches will be detected as changed.
// New patches can easily be added to the end of the chain, but patches cannot
// be inserted in the middle of the chain.
