// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func shaSumFile(c *gc.C, fileToSum string) string {
	f, err := os.Open(fileToSum)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	shahash := sha1.New()
	_, err = io.Copy(shahash, f)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}
