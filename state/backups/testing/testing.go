// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// SHA1SumFile returns the RFC 3230 SHA hash of the file.
func SHA1SumFile(c *gc.C, file *os.File) string {
	shahash := sha1.New()
	_, err := io.Copy(shahash, file)
	c.Assert(err, jc.ErrorIsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}
