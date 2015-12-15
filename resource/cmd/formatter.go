// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type charmResourcesFormatter struct {
	resources []charmresource.Resource
}

func newCharmResourcesFormatter(resources []charmresource.Resource) *charmResourcesFormatter {
	// Note that unlike the "juju status" code, we don't worry
	// about "compatVersion".
	crf := charmResourcesFormatter{
		resources: resources,
	}
	return &crf
}

func (crf *charmResourcesFormatter) format() []FormattedCharmResource {
	if crf.resources == nil {
		return nil
	}

	var formatted []FormattedCharmResource
	for _, res := range crf.resources {
		formatted = append(formatted, FormatCharmResource(res))
	}
	return formatted
}

// FormatCharmResource converts the resource info into a FormattedCharmResource.
func FormatCharmResource(res charmresource.Resource) FormattedCharmResource {
	return FormattedCharmResource{
		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Comment:     res.Comment,
		Revision:    res.Revision,
		Origin:      res.Origin.String(),
		Fingerprint: res.Fingerprint.String(),
	}
}
