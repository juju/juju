// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"os"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// CharmMeta describes methods that inform charm operation.
type CharmMeta interface {
	Meta() *Meta
	Manifest() *Manifest
}

// The Charm interface is implemented by any type that
// may be handled as a charm.
type Charm interface {
	CharmMeta
	Config() *Config
	Actions() *Actions
	Revision() int
}

// ReadCharm reads a Charm from path, which can point to either a charm archive or a
// charm directory.
func ReadCharm(path string) (charm Charm, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if info.IsDir() {
		charm, err = ReadCharmDir(path)
	} else {
		charm, err = ReadCharmArchive(path)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	return charm, errors.Trace(CheckMeta(charm))
}

// FormatSelectionReason represents the reason for a format version selection.
type FormatSelectionReason = string

const (
	// SelectionManifest states that it found a manifest.
	SelectionManifest FormatSelectionReason = "manifest"
	// SelectionBases states that there was at least 1 base.
	SelectionBases FormatSelectionReason = "bases"
	// SelectionContainers states that there was at least 1 container.
	SelectionContainers FormatSelectionReason = "containers"
)

var (
	// formatV2Set defines what in reality is a v2 metadata.
	formatV2Set = set.NewStrings(SelectionBases, SelectionContainers)
)

// metaFormatReasons returns the format and why the selection was done. We can
// then inspect the reasons to understand the reasoning.
func metaFormatReasons(ch CharmMeta) (Format, []FormatSelectionReason) {
	manifest := ch.Manifest()

	// To better inform users of why a metadata selection was preferred over
	// another, we deduce why a format is selected over another.
	reasons := set.NewStrings()
	if manifest != nil {
		reasons.Add(SelectionManifest)
		if len(manifest.Bases) > 0 {
			reasons.Add(SelectionBases)
		}
	}

	if len(ch.Meta().Containers) > 0 {
		reasons.Add(SelectionContainers)
	}

	// To be a format v1 you can have no series with no bases or containers, or
	// just have a series slice.
	format := FormatV1
	if reasons.Intersection(formatV2Set).Size() > 0 {
		format = FormatV2
	}

	return format, reasons.SortedValues()
}

// MetaFormat returns the underlying format from checking the charm for the
// right values.
func MetaFormat(ch CharmMeta) Format {
	format, _ := metaFormatReasons(ch)
	return format
}

// CheckMeta determines the version of the metadata used by this charm,
// then checks that it is valid as appropriate.
func CheckMeta(ch CharmMeta) error {
	format, reasons := metaFormatReasons(ch)
	return ch.Meta().Check(format, reasons...)
}
