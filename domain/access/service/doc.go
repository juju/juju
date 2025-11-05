// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for managing users and permissions
// in Juju, implementing authentication and authorization functionality.
//
// The service layer coordinates between API facades and the persistence layer,
// providing methods for:
//   - User management (add, remove, update, authenticate)
//   - Permission management (grant, revoke, check access)
//   - External user handling and everyone@external group semantics
package service
