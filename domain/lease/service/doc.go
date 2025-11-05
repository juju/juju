// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for lease management in Juju.
//
// The service layer implements operations for:
//   - Creating and claiming leases
//   - Extending lease durations
//   - Expiring and revoking leases
//   - Tracking lease holders and responsibilities
//   - Coordinating singular responsibility among multiple instances
package service
