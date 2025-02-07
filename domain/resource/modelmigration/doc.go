// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelmigration handles import and export of resources during
// migration.
//
// The migration of resource blobs is not handled by this package, this is done
// during the "Upload Binaries" stage of the migration. This package migrates
// the information about how the resource is used on the controller.
//
// When migrating from 3.6 to 4, certain pieces of information are not migrated
// due to schema constrains in 4.0:
//  - The repository resources available on charmhub.
//  - The information pertaining to the retriever of the resource.

package modelmigration
