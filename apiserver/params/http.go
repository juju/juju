// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DigestAlgorithm is one of the values in the IANA registry. See
// RFC 3230 and 5843.
//
// Note that currently Juju does not conform to the standard.
// It stores a hexadecimal SHA256 value in the Digest header,
// but the above RFCs specify SHA-256 and a base64-encoded
// value for this.
// TODO fix that. https://bugs.launchpad.net/juju-core/+bug/1503992
type DigestAlgorithm string

const (
	// DigestSHA is the HTTP digest algorithm value used in juju's HTTP code.
	DigestSHA DigestAlgorithm = "SHA"

	// The values used for content-type in juju's direct HTTP code:

	// ContentTypeJSON is the HTTP content-type value used for JSON content.
	ContentTypeJSON = "application/json"

	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"

	// ContentTypeJS is the HTTP content-type value used for javascript.
	ContentTypeJS = "application/javascript"

	// ContentTypeXJS is the outdated HTTP content-type value used for javascript.
	ContentTypeXJS = "application/x-javascript"
)
