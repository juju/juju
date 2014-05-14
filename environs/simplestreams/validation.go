// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

// MetadataValidator instances can provide parameters used to query simplestreams
// metadata to find information for the specified parameters. If region is "",
// then the implementation may use its own default region if it has one,
// or else returns an error.
type MetadataValidator interface {
	MetadataLookupParams(region string) (*MetadataLookupParams, error)
}

type MetadataLookupParams struct {
	Region        string
	Series        string
	Architectures []string
	Endpoint      string
	Sources       []DataSource
	Stream        string
}
