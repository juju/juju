// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jwt provides an authenticator which accepts a jwt as
// the means to declare the user wanting access and the permissions
// they have. It is used to authenticate Juju login requests against
// the api endpoint, and raw http requests.
//
// This mechanism is used when a Juju Controller is registered with JAAS.
// JAAS has its own permissions model with finer grained RBAC permissions
// than Juju supports. JAAS will perform the authentication step and present
// to Juju a jwt with the username and permissions matching what Juju supports.
//
// Juju uses a [github.com/juju/juju/apiserver/authentication.PermissionDelegator]
// instance to look up what permissions a user has. For non jwt scenarios, these
// come from the Juju permissions model; when a jwt is used these come from the jwt
// and the Juju permission model is empty.
//
// To use this authentication mechanism, the controller is bootstrapped with the
// 'login-token-refresh-url' attribute set to the JAAS jwt refresh endpoint.
//
// The other implication of configuring a controller to use this authentication
// mechanism is that the discharge endpoint for cross model relation macaroons
// is also set to point to JAAS, so that JAAS is the service that also validates
// permissions for consuming offers.

package jwt
