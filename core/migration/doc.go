// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

// Package migration provides a way to perform migrations on the database.
// Migrations are a collection of operations that can be performed as a single
// unit. This is not atomic, but it does allow for a rollback of the entire
// migration if any operation fails.
// Each operation can either be an import or an export operation. The
// description package can been read or written to perform the migration.
