// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/simplestreams"
)

// ToolsMetadataLookupParams is used to query metadata for matching tools.
type ToolsMetadataLookupParams struct {
	simplestreams.MetadataLookupParams
	Version string
	Major   int
	Minor   int
}

// ValidateToolsMetadata attempts to load tools metadata for the specified cloud attributes and returns
// any tools versions found, or an error if the metadata could not be loaded.
func ValidateToolsMetadata(ctx context.Context, ss SimplestreamsFetcher, params *ToolsMetadataLookupParams) ([]string, *simplestreams.ResolveInfo, error) {
	if len(params.Sources) == 0 {
		return nil, nil, fmt.Errorf("required parameter sources not specified")
	}
	if params.Version == "" && params.Major == 0 {
		params.Version = jujuversion.Current.String()
	}
	var toolsConstraint *ToolsConstraint
	if params.Version == "" {
		toolsConstraint = NewGeneralToolsConstraint(params.Major, params.Minor, simplestreams.LookupParams{
			CloudSpec: simplestreams.CloudSpec{
				Region:   params.Region,
				Endpoint: params.Endpoint,
			},
			Stream:   params.Stream,
			Releases: []string{params.Release},
			Arches:   params.Architectures,
		})
	} else {
		versNum, err := semversion.Parse(params.Version)
		if err != nil {
			return nil, nil, err
		}
		toolsConstraint = NewVersionedToolsConstraint(versNum, simplestreams.LookupParams{
			CloudSpec: simplestreams.CloudSpec{
				Region:   params.Region,
				Endpoint: params.Endpoint,
			},
			Stream:   params.Stream,
			Releases: []string{params.Release},
			Arches:   params.Architectures,
		})
	}
	matchingTools, resolveInfo, err := Fetch(ctx, ss, params.Sources, toolsConstraint)
	if err != nil {
		return nil, resolveInfo, err
	}
	if len(matchingTools) == 0 {
		return nil, resolveInfo, fmt.Errorf("no matching agent binaries found for constraint %+v", toolsConstraint)
	}
	versions := make([]string, len(matchingTools))
	for i, tm := range matchingTools {
		vers := semversion.Binary{
			Number:  semversion.MustParse(tm.Version),
			Release: tm.Release,
			Arch:    tm.Arch,
		}
		versions[i] = vers.String()
	}
	return versions, resolveInfo, nil
}
