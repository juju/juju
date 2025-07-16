// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// ControllerAgentInfo holds network/auth information needed by a controller.
type ControllerAgentInfo struct {
	APIPort        int
	Cert           string
	PrivateKey     string
	CAPrivateKey   string
	SystemIdentity string
}
