// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"encoding/base64"
	"fmt"
)

// DigestAlgorithm is one of the values in the IANA registry. See
// RFC 3230 and 5843.
type DigestAlgorithm string

const (
	// DigestSHA is the HTTP digest algorithm value used in juju's HTTP code.
	DigestSHA256 DigestAlgorithm = "SHA-256"

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

// EncodeChecksum base64 encodes a sha256 checksum according to RFC 4648 and
// returns a value that can be added to the "Digest" http header.
func EncodeChecksum(checksum string) string {
	return fmt.Sprintf("%s=%s", DigestSHA256, base64.StdEncoding.EncodeToString([]byte(checksum)))
}
