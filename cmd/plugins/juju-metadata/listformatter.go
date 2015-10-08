// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

func formatMetadataListTabular(value interface{}) ([]byte, error) {
	metadata, ok := value.([]MetadataInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", metadata, value)
	}
	return formatMetadataTabular(metadata)
}

// formatMetadataTabular returns a tabular summary of cloud image metadata.
func formatMetadataTabular(metadata []MetadataInfo) ([]byte, error) {
	var out bytes.Buffer

	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}
	print("SOURCE", "SERIES", "ARCH", "REGION", "IMAGE-ID", "STREAM", "VIRT-TYPE", "STORAGE-TYPE")

	for _, m := range metadata {
		print(m.Source, m.Series, m.Arch, m.Region, m.ImageId, m.Stream, m.VirtType, m.RootStorageType)
	}
	tw.Flush()

	return out.Bytes(), nil
}
