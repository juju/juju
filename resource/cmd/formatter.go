// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
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
		Type:        convType(res.Type),
		Path:        res.Path,
		Comment:     res.Comment,
		Revision:    res.Revision,
		Origin:      convOrigin(res.Origin),
		Fingerprint: res.Fingerprint.String(),
	}
}

// FormatSvcResource converts the resource info into a FormattedServiceResource.
func FormatSvcResource(res resource.Resource) FormattedSvcResource {
	return FormattedSvcResource{
		Name:        res.Name,
		Type:        convType(res.Type),
		Path:        res.Path,
		Used:        !res.Timestamp.IsZero(),
		Revision:    res.Revision,
		Origin:      convOrigin(res.Origin),
		Fingerprint: res.Fingerprint.String(),
		Comment:     res.Comment,
		Timestamp:   res.Timestamp,
		Username:    res.Username,
	}
}

func convOrigin(origin charmresource.Origin) Origin {
	switch origin {
	case charmresource.OriginStore:
		return OriginStore
	case charmresource.OriginUpload:
		return OriginUpload
	default:
		return OriginUnknown
	}
}

func convType(typ charmresource.Type) DataType {
	switch typ {
	case charmresource.TypeFile:
		return DataTypeFile
	default:
		return DataTypeUnknown
	}
}
