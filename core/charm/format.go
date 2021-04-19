// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/charm/v8"

// MetadataFormat of the parsed charm.
type MetadataFormat int

// MetadataFormat are the different versions of charm metadata supported.
const (
	FormatUnknown MetadataFormat = iota
	FormatV1      MetadataFormat = iota
	FormatV2      MetadataFormat = iota
)

type CharmManifest interface {
	Manifest() *charm.Manifest
}

// Given a charm, what format is it in?
func Format(ch CharmManifest) MetadataFormat {
	if ch.Manifest() == nil || len(ch.Manifest().Bases) == 0 {
		return FormatV1
	}
	return FormatV2
}
