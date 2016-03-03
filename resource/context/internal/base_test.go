// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"io"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
)

func newResource(c *gc.C, stub *testing.Stub, name, content string) (resource.Resource, io.ReadCloser) {
	opened := resourcetesting.NewResource(c, stub, name, "a-service", content)
	res := opened.Resource
	if content != "" {
		return res, opened.ReadCloser
	}
	res.Username = ""
	res.Timestamp = time.Time{}
	return res, nil
}
