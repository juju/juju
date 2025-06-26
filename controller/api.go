// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// StateServingInfo holds network/auth information needed by a controller.
type StateServingInfo struct {
	APIPort      int
	StatePort    int
	Cert         string
	PrivateKey   string
	CAPrivateKey string
	// this will be passed as the KeyFile argument to MongoDB
	SharedSecret   string
	SystemIdentity string
}
