// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for managing clouds in Juju.
//
// The service layer implements operations for:
//   - Adding, updating, and removing cloud definitions
//   - Managing cloud regions and their availability
//   - Handling cloud authentication types and endpoints
//   - Validating cloud configurations
//   - Coordinating cloud operations during controller bootstrap
package service
