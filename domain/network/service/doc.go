// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for network management in Juju.
//
// The service layer implements operations for:
//   - Managing spaces (logical groupings of subnets)
//   - Handling subnet discovery and configuration
//   - Coordinating network isolation for applications
//   - Managing space constraints and bindings
//   - Importing network information from providers (especially MAAS)
package service
