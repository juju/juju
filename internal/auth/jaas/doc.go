// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jaas provides an authenticator implementation for JWT tokens received
// from a trusted JAAS controllers. All external users that come from a trusted
// JAAS controller will be created in the controller database when the
// authentication result is checked by the caller.
//
// History:
// The JAAS authenticator was first introduced into Juju in version 3. It was
// done as a way to demonstrate that JAAS can extend Juju further by adding
// enterprise OAUTH2 support and high level management of Juju controller users
// across many controllers.
package jaas
