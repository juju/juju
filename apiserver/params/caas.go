// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CAASConnectionConfig holds CAAS connection config.
type CAASConnectionConfig struct {
	Endpoint       string   `json:"endpoint"`
	CACertificates []string `json:"ca-certificates,omitempty"`
	CertData       []byte   `json:"cert-data,omitempty"`
	KeyData        []byte   `json:"key-data,omitempty"`
	Username       string   `json:"username"`
	Password       string   `json:"password"`
}
