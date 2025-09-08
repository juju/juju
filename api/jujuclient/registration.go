// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

// RegistrationInfo contains the user/controller registration information
// printed by "juju add-user", and consumed by "juju register".
type RegistrationInfo struct {
	// User is the user name to log in as.
	User string

	// Addrs contains the "host:port" addresses of the Juju
	// controller.
	Addrs []string

	// SecretKey contains the secret key to use when encrypting
	// and decrypting registration requests and responses.
	SecretKey []byte

	// ControllerName contains the name that the controller has for the
	// caller of "juju add-user" that will be used to suggest a name for
	// the caller of "juju register".
	ControllerName string

	// ProxyConfig is a config around a real proxier interface that should
	// be used to connect to the controller.
	ProxyConfig string
}
