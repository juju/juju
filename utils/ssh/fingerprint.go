// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build !windows

package ssh

import (
	"bytes"
	"crypto/md5"
	"fmt"
)

// KeyFingerprint returns the fingerprint and comment for the specified key
// in authorized_key format.
func KeyFingerprint(key string) (fingerprint, comment string, err error) {
	ak, err := ParseAuthorisedKey(key)
	if err != nil {
		return "", "", fmt.Errorf("generating key fingerprint: %v", err)
	}
	sum := md5.Sum(ak.Key)
	var buf bytes.Buffer
	for i, b := range sum {
		if i > 0 {
			buf.WriteByte(':')
		}
		buf.WriteString(fmt.Sprintf("%02x", b))
	}
	return buf.String(), ak.Comment, nil
}
