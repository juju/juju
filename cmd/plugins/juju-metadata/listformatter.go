// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/output"
)

func formatMetadataListTabular(writer io.Writer, value interface{}) error {
	metadata, ok := value.([]MetadataInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", metadata, value)
	}
	formatMetadataTabular(writer, metadata)
	return nil
}

// formatMetadataTabular writes a tabular summary of cloud image metadata.
func formatMetadataTabular(writer io.Writer, metadata []MetadataInfo) {
	tw := output.TabWriter(writer)
	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}
	print("SOURCE", "SERIES", "ARCH", "REGION", "IMAGE-ID", "STREAM", "VIRT-TYPE", "STORAGE-TYPE")

	for _, m := range metadata {
		print(m.Source, m.Series, m.Arch, m.Region, m.ImageId, m.Stream, m.VirtType, m.RootStorageType)
	}
	tw.Flush()
}
