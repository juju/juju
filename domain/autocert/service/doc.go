// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for autocert cache management.
//
// The service layer implements operations for:
//   - Storing and retrieving TLS certificates
//   - Managing certificate cache operations
//   - Coordinating with the acme/autocert package
package service
