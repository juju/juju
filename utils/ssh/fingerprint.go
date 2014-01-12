// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"crypto/md5"
	"fmt"
)

// KeyFingerprint returns the fingerprint and comment for the specified key
// in authorized_key format. Fingerprints are generated according to RFC4716.
// See ttp://www.ietf.org/rfc/rfc4716.txt, section 4.
func KeyFingerprint(key string) (fingerprint, comment string, err error) {
	ak, err := ParseAuthorisedKey(key)
	if err != nil {
		return "", "", fmt.Errorf("generating key fingerprint: %v", err)
	}
	hash := md5.New()
	hash.Write(ak.Key)
	sum := hash.Sum(nil)
	var buf bytes.Buffer
	for i := 0; i < hash.Size(); i++ {
		if i > 0 {
			buf.WriteByte(':')
		}
		buf.WriteString(fmt.Sprintf("%02x", sum[i]))
	}
	return buf.String(), ak.Comment, nil
}
