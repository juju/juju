// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testcharms holds a corpus of charms
// for testing.
package testcharms

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/charmrepo.v2-unstable/testing"
)

// Repo provides access to the test charm repository.
var Repo = testing.NewRepo("charm-repo", "quantal")

// UploadCharmWithMeta pushes a new charm to the charmstore.
// The uploaded charm takes the supplied charmURL with metadata.yaml and metrics.yaml
// to define the charm, rather than relying on the charm to exist on disk.
// This allows you to create charm definitions directly in yaml and have them uploaded
// here for us in tests.
//
// For convenience the charm is also made public
func UploadCharmWithMeta(c *gc.C, client *csclient.Client, charmURL, meta, metrics string, revision int) (*charm.URL, charm.Charm) {
	ch := testing.NewCharm(c, testing.CharmSpec{
		Meta:     meta,
		Metrics:  metrics,
		Revision: revision,
	})
	chURL, err := client.UploadCharm(charm.MustParseURL(charmURL), ch)
	c.Assert(err, jc.ErrorIsNil)
	SetPublic(c, client, chURL)
	return chURL, ch
}

// UploadCharm uploads a charm using the given charm store client, and returns
// the resulting charm URL and charm.
//
// It also adds any required resources that haven't already been uploaded
// with the content "<resourcename> content".
func UploadCharm(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Charm) {
	id := charm.MustParseURL(url)
	promulgatedRevision := -1
	if id.User == "" {
		// We still need a user even if we are uploading a promulgated charm.
		id.User = "who"
		promulgatedRevision = id.Revision
	}
	ch := Repo.CharmArchive(c.MkDir(), name)

	// Upload the charm.
	err := client.UploadCharmWithRevision(id, ch, promulgatedRevision)
	c.Assert(err, jc.ErrorIsNil)

	// Upload any resources required for publishing.
	var resources map[string]int
	if len(ch.Meta().Resources) > 0 {
		// The charm has resources.
		// Ensure that all the required resources are uploaded
		// before we publish.
		resources = make(map[string]int)
		current, err := client.WithChannel(params.UnpublishedChannel).ListResources(id)
		c.Assert(err, gc.IsNil)
		for _, r := range current {
			if r.Revision == -1 {
				// The resource doesn't exist so upload one.
				_, err := client.UploadResource(id, r.Name, "", strings.NewReader(r.Name+" content"))
				c.Assert(err, jc.ErrorIsNil)
				r.Revision = 0
			}
			resources[r.Name] = r.Revision
		}
	}

	SetPublicWithResources(c, client, id, resources)

	return id, ch
}

// UploadCharmMultiSeries uploads a charm with revision using the given charm store client,
// and returns the resulting charm URL and charm. This API caters for new multi-series charms
// which do not specify a series in the URL.
func UploadCharmMultiSeries(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Charm) {
	id := charm.MustParseURL(url)
	if id.User == "" {
		// We still need a user even if we are uploading a promulgated charm.
		id.User = "who"
	}
	ch := Repo.CharmArchive(c.MkDir(), name)

	// Upload the charm.
	curl, err := client.UploadCharm(id, ch)
	c.Assert(err, jc.ErrorIsNil)

	SetPublic(c, client, curl)

	// Return the charm and its URL.
	return curl, ch
}

// UploadBundle uploads a bundle using the given charm store client, and
// returns the resulting bundle URL and bundle.
func UploadBundle(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Bundle) {
	id := charm.MustParseURL(url)
	promulgatedRevision := -1
	if id.User == "" {
		// We still need a user even if we are uploading a promulgated bundle.
		id.User = "who"
		promulgatedRevision = id.Revision
	}
	b := Repo.BundleArchive(c.MkDir(), name)

	// Upload the bundle.
	err := client.UploadBundleWithRevision(id, b, promulgatedRevision)
	c.Assert(err, jc.ErrorIsNil)

	SetPublic(c, client, id)

	// Return the bundle and its URL.
	return id, b
}

// SetPublicWithResources sets the charm or bundle with the given id to be
// published with global read permissions to the stable channel.
//
// The named resources with their associated revision
// numbers are also published.
func SetPublicWithResources(c *gc.C, client *csclient.Client, id *charm.URL, resources map[string]int) {
	// Publish to the stable channel.
	err := client.Publish(id, []params.Channel{params.StableChannel}, resources)
	c.Assert(err, jc.ErrorIsNil)

	// Allow stable read permissions to everyone.
	err = client.WithChannel(params.StableChannel).Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone})
	c.Assert(err, jc.ErrorIsNil)
}

// SetPublic sets the charm or bundle with the given id to be
// published with global read permissions to the stable channel.
func SetPublic(c *gc.C, client *csclient.Client, id *charm.URL) {
	SetPublicWithResources(c, client, id, nil)
}
