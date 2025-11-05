// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package macaroon provides domain types and services for macaroon-based
// authentication in Juju.
//
// Macaroons are bearer tokens that can be used to authenticate API requests.
// They support attenuating caveats (restrictions) and can be delegated to
// third parties for limited access.
//
// # Key Concepts
//
// Macaroons in Juju:
//   - Store authentication roots for token validation
//   - Support time-based and capability-based caveats
//   - Enable delegated authentication for external access
//   - Integrate with the access domain for permission checking
//
// The macaroon domain coordinates with the access control system to provide
// flexible, secure authentication for Juju operations.
package macaroon
