// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for application management.
//
// The service layer implements operations for:
//   - Creating and deploying applications from charms
//   - Managing application configuration and constraints
//   - Handling application lifecycle (IAAS, CAAS, and synthetic CMR apps)
//   - Scaling applications (adding/removing units)
//   - Managing application resources and storage
//   - Coordinating with the charm and relation domains
package service
