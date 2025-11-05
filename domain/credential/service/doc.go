// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for credential management.
//
// The service layer implements operations for:
//   - Adding and updating cloud credentials
//   - Validating credential data
//   - Marking credentials as invalid or revoking them
//   - Managing credential-model associations
//   - Coordinating credential validation checks
package service
