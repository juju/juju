// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func shaSumFile(c *gc.C, file *os.File) string {
	shahash := sha1.New()
	_, err := io.Copy(shahash, file)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}
