// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"sort"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/testcharms"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

type ResourcesBundleSuite struct {
	application.BundleDeployCharmStoreSuite
}

var _ = gc.Suite(&ResourcesBundleSuite{})

func (s *ResourcesBundleSuite) TestDeployBundleResources(c *gc.C) {
	testcharms.UploadCharm(c, s.Client(), "trusty/starsay-42", "starsay")
	bundleMeta := `
        applications:
            starsay:
                charm: cs:starsay
                num_units: 1
                resources:
                    store-resource: 0
                    install-resource: 0
                    upload-resource: 0
    `
	output, err := s.DeployBundleYAML(c, bundleMeta)
	c.Assert(err, jc.ErrorIsNil)

	lines := strings.Split(output, "\n")
	expectedLines := strings.Split(strings.TrimSpace(`
Deploying charm "cs:trusty/starsay-42"
added resource install-resource
added resource store-resource
added resource upload-resource
Deploy of bundle completed.
    `), "\n")
	c.Check(lines, gc.HasLen, len(expectedLines))
	c.Check(lines[0], gc.Equals, expectedLines[0])
	// The "added resource" lines are checked after we sort since
	// the ordering of those lines is unknown.
	sort.Strings(lines)
	sort.Strings(expectedLines)
	c.Check(lines, jc.DeepEquals, expectedLines)
	s.checkResources(c, "starsay", []resource.Resource{{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "install-resource",
				Type:        charmresource.TypeFile,
				Path:        "gotta-have-it.txt",
				Description: "get things started",
			},
			Origin:      charmresource.OriginStore,
			Revision:    0,
			Fingerprint: resourceHash("install-resource content"),
			Size:        int64(len("install-resource content")),
		},
		ID:            "starsay/install-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "store-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Fingerprint: resourceHash("store-resource content"),
			Size:        int64(len("store-resource content")),
			Revision:    0,
		},
		ID:            "starsay/store-resource",
		ApplicationID: "starsay",
	}, {
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "upload-resource",
				Type:        charmresource.TypeFile,
				Path:        "somename.xml",
				Description: "Who uses xml anymore?",
			},
			Origin:      charmresource.OriginStore,
			Fingerprint: resourceHash("upload-resource content"),
			Size:        int64(len("upload-resource content")),
			Revision:    0,
		},
		ID:            "starsay/upload-resource",
		ApplicationID: "starsay",
	}})
}

func (s *ResourcesBundleSuite) checkResources(c *gc.C, serviceName string, expected []resource.Resource) {
	_, err := s.State.Application("starsay")
	c.Check(err, jc.ErrorIsNil)
	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	svcResources, err := st.ListResources("starsay")
	c.Assert(err, jc.ErrorIsNil)
	resources := svcResources.Resources
	resource.Sort(resources)
	c.Assert(resources, jc.DeepEquals, expected)
}
