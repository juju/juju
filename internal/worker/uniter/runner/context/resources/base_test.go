// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io"
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

func newCharmResource(c *tc.C, stub *testing.Stub, name, content string, resType charmresource.Type) (resource.Resource, io.ReadCloser) {
	opened := resourcetesting.NewResource(c, stub, name, "a-application", content)
	res := opened.Resource
	res.Type = resType
	if content != "" {
		return res, opened.ReadCloser
	}
	res.RetrievedBy = ""
	res.Timestamp = time.Time{}
	return res, nil
}

func newResource(c *tc.C, stub *testing.Stub, name, content string) (resource.Resource, io.ReadCloser) {
	return newCharmResource(c, stub, name, content, charmresource.TypeFile)
}

func newDockerResource(c *tc.C, stub *testing.Stub, name, content string) (resource.Resource, io.ReadCloser) {
	return newCharmResource(c, stub, name, content, charmresource.TypeContainerImage)
}
