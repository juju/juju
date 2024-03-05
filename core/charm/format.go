// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/charm/v13"

// MetadataFormat of the parsed charm.
type MetadataFormat int

// MetadataFormat are the different versions of charm metadata supported.
const (
	FormatUnknown MetadataFormat = iota
	FormatV1      MetadataFormat = iota
	FormatV2      MetadataFormat = iota
)

// Format returns the metadata format for a given charm.
func Format(ch charm.CharmMeta) MetadataFormat {
	m := ch.Manifest()
	if m == nil || len(m.Bases) == 0 || len(ch.Meta().Series) > 0 {
		return FormatV1
	}
	return FormatV2
}
