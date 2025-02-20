// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jwt provides an authentication and authorisation mechanism whereby
// a JAAS-registered Juju controller can accept a JAAS JWT (JSON Web Token)
// as the means to declare a user and their permissions.
//
// This mechanism kicks in if, after setting up the JWT in JAAS, you bootstrap your
// controller with the 'login-token-refresh-url' controller config set to
// the JAAS JWT refresh endpoint.
//
// The JWTs are parsed by a separate object, see [github.com/juju/juju/worker/jwtparser].
//
// # Authentication
//
// Because it contains a username vetted by JAAS, the JWT is used to authenticate
// Juju login requests against the API endpoint, and raw HTTP requests.
//
// # Authorisation
//
// Juju uses a [github.com/juju/juju/apiserver/authentication.PermissionDelegator]
// instance to look up what permissions a user has. For non-JWT scenarios, these
// come from the Juju permissions model. However, when a JWT is used, the Juju
// permission model is empty, and the permissions are retrieved from the JWT.
// JAAS provides a finer-grained permission model than Juju, and so constructs the
// JWT to contain permissions matching those which Juju understands.
//
// Using a JWT alters slightly the macaroon discharge configuration used when
// authorising a cross model relations operation. Instead of the discharge endpoint
// being on the offering controller, it instead points to JAAS, which uses
// its permissions model to validate access for consuming offers.

package jwt
