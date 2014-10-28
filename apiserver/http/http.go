// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

// DigestAlgorithm is one of the values in the IANA registry. See
// RFC 3230 and 5843.
type DigestAlgorithm string

const (
	DIGEST_SHA DigestAlgorithm = "SHA"

	CTYPE_JSON = "application/json"
	CTYPE_RAW  = "application/octet-stream"
)
