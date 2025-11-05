// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for secret management.
//
// The service layer implements operations for:
//   - Creating and updating secrets
//   - Managing secret revisions and content
//   - Granting and revoking secret access
//   - Rotating secrets according to policies
//   - Coordinating with secret backends
package service
