// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

// DigestAlgorithm is one of the values in the IANA registry. See
// RFC 3230 and 5843.
type DigestAlgorithm string

const (
	// DigestSHA is the HTTP digest algorithm value used in juju's HTTP code.
	DigestSHA DigestAlgorithm = "SHA"
)
