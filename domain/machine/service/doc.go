// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides the business logic for machine management in Juju.
//
// The service layer implements operations for:
//   - Creating and removing machines
//   - Managing machine lifecycle (alive, dying, dead)
//   - Setting and retrieving machine cloud instance information
//   - Managing hardware characteristics and availability zones
//   - Handling machine agent versions and reboot requirements
//   - Coordinating with provisioners for machine setup
package service
