// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

func newFullCharmResource(name string) charmresource.Resource {
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:    name,
			Type:    charmresource.TypeFile,
			Path:    name + ".tgz",
			Comment: "you need it",
		},
		Revision:    1,
		Fingerprint: "chdec737riyg2kqja3yh",
	}
}

func newFullInfo(name string) resource.Info {
	return resource.Info{
		Resource: newFullCharmResource(name),
		Origin:   resource.OriginKindUpload,
	}
}

func newFullResource(name string) resource.Resource {
	return resource.Resource{
		Info:      newFullInfo(name),
		Username:  "a-user",
		Timestamp: time.Now(),
	}
}
