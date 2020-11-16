// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type ResourceRevision struct {
	Download ResourceDownload `json:"download"`
	Name     string           `json:"name"`
	Revision int              `json:"revision"`
	Type     string           `json:"type"`
}

type ResourceDownload struct {
	HashSHA256  string `json:"hash-sha256"`
	HashSHA3384 string `json:"hash-sha3-384"`
	HashSHA384  string `json:"hash-sha384"`
	HashSHA512  string `json:"hash-sha512"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
}
