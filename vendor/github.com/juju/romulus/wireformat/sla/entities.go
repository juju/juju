// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The sla package implements wireformats for the sla service.
package sla

import (
	"gopkg.in/macaroon.v2-unstable"
)

// SLARequest defines the json used to post to sla service.
type SLARequest struct {
	ModelUUID string `json:"model"`
	Level     string `json:"sla"`
	Budget    string `json:"budget"`
}

// SLAResponse defines the json response when an sla is set.
type SLAResponse struct {
	Owner       string             `json:"owner"`
	Credentials *macaroon.Macaroon `json:"credentials"`
	Message     string             `json:"message,omitempty"`
}
