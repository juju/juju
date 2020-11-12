// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type ResourcesResponse struct {
	Revisions []ResourceRevision `json:"revisions"`
}

type ResourceRevision struct {
	Download ResourceDownload `json:"download"`
	Name     string           `json:"name"`
	Revision int              `json:"revision"`
	Type     string           `json:"type"`
}

type ResourceDownload struct {
	// As of 12-Nov-2020, the json for HashSHA256 is different
	// between the resource version of the download object and
	// the info etc versions.  This object also has more hash types.
	HashSHA256 string `json:"hash-sha256"`
	Size       int    `json:"size"`
	URL        string `json:"url"`
}
