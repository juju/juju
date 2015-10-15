// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testcharms holds a corpus of charms
// for testing.
package testcharms

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v1/csclient"
	"gopkg.in/juju/charmrepo.v1/csclient/params"
	"gopkg.in/juju/charmrepo.v1/testing"
)

// Repo provides access to the test charm repository.
var Repo = testing.NewRepo("charm-repo", "quantal")

// UploadCharm uploads a charm using the given charm store client, and returns
// the resulting charm URL and charm.
func UploadCharm(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Charm) {
	id := charm.MustParseReference(url)
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

	// Allow read permissions to everyone.
	err = client.Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone})
	c.Assert(err, jc.ErrorIsNil)

	// Return the charm and its URL.
	curl, err := id.URL("")
	c.Assert(err, jc.ErrorIsNil)
	return curl, ch
}

// UploadBundle uploads a bundle using the given charm store client, and
// returns the resulting bundle URL and bundle.
func UploadBundle(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Bundle) {
	id := charm.MustParseReference(url)
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

	// Allow read permissions to everyone.
	err = client.Put("/"+id.Path()+"/meta/perm/read", []string{params.Everyone})
	c.Assert(err, jc.ErrorIsNil)

	// Return the bundle and its URL.
	burl, err := id.URL("")
	c.Assert(err, jc.ErrorIsNil)
	return burl, b
}
