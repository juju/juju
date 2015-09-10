// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// formatFilesystemListTabular returns a tabular summary of filesystem instances.
func formatFilesystemListTabular(value interface{}) ([]byte, error) {
	infos, ok := value.(map[string]map[string]map[string]FilesystemInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", infos, value)
	}
	return formatFilesystemListTabularTyped(infos), nil
}

func formatFilesystemListTabularTyped(infos map[string]map[string]map[string]FilesystemInfo) []byte {
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
	print("MACHINE", "UNIT", "STORAGE", "FILESYSTEM", "VOLUME", "ID", "MOUNTPOINT", "SIZE", "STATE", "MESSAGE")

	// 1. sort by machines
	machines := set.NewStrings()
	for machine := range infos {
		if !machines.Contains(machine) {
			machines.Add(machine)
		}
	}
	for _, machine := range machines.SortedValues() {
		machineUnits := infos[machine]

		// 2. sort by unit
		units := set.NewStrings()
		for unit := range machineUnits {
			if !units.Contains(unit) {
				units.Add(unit)
			}
		}
		for _, unit := range units.SortedValues() {
			unitStorages := machineUnits[unit]

			// 3. sort by storage
			storages := set.NewStrings()
			for storage := range unitStorages {
				if !storages.Contains(storage) {
					storages.Add(storage)
				}
			}
			for _, storage := range storages.SortedValues() {
				info := unitStorages[storage]
				var size string
				if info.Size > 0 {
					size = humanize.IBytes(info.Size * humanize.MiByte)
				}
				print(
					machine, unit, storage,
					info.Filesystem, info.Volume,
					info.FilesystemId,
					info.MountPoint, size,
					string(info.Status.Current), info.Status.Message,
				)
			}
		}
	}
	tw.Flush()
	return out.Bytes()
}
