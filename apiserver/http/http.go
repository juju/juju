// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

// DigestAlgorithm is one of the values in the IANA registry. See
// RFC 3230 and 5843.
type DigestAlgorithm string

const (
	// DigestSHA is the HTTP digest algorithm value used in juju's HTTP code.
	DigestSHA DigestAlgorithm = "SHA"

	// The values used for content-type in juju's direct HTTP code:

	// CTypeJSON is the HTTP content-type value used for JSON content.
	CTypeJSON = "application/json"
	// CTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	CTypeRaw = "application/octet-stream"
)
