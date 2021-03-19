// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

// AgentMetadataValidator instances can provide parameters used to query simplestreams
// metadata to find agent information for the specified parameters. If region is "",
// then the implementation may use its own default region if it has one,
// or else returns an error.
type AgentMetadataValidator interface {
	AgentMetadataLookupParams(region string) (*MetadataLookupParams, error)
}

// ImageMetadataValidator instances can provide parameters used to query simplestreams
// metadata to find agent information for the specified parameters. If region is "",
// then the implementation may use its own default region if it has one,
// or else returns an error.
type ImageMetadataValidator interface {
	ImageMetadataLookupParams(region string) (*MetadataLookupParams, error)
}

type MetadataLookupParams struct {
	Region        string
	Release       string
	Architectures []string
	Endpoint      string
	Sources       []DataSource
	Stream        string
}
