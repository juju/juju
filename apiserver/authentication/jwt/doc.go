// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jwt provides an authentication mechanism whereby a JAAS-registered
// Juju controller can accept a JAAS JWT (JSON Web Token) as the means to
// declare a user and their permissions. The JWT is used to authenticate Juju
// login requests against the api endpoint, and raw http requests.
//
// This mechanism kicks in if, after setting up the JWT in JAAS, you bootstrap your
// controller with the 'login-token-refresh-url' controller config set to
// the JAAS JWT refresh endpoint.
//
// Juju uses a [github.com/juju/juju/apiserver/authentication.PermissionDelegator]
// instance to look up what permissions a user has. For non-JWT scenarios, these
// come from the Juju permissions model. However, when a JWT is used, the Juju
// permission model is empty, and the permissions are retrieved from the JWT.
// JAAS provides a finer-grained permission model than Juju, and so constructs the
// JWT to contain permissions matching those which Juju understands.
//
// Configuring a Juju controller to use a JWT for authentication means that the
// discharge of cross model relation macaroons is performed by JAAS, which uses
// its permissions model to validate access for consuming offers.

package jwt
