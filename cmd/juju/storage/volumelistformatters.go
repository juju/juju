package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
)

// formatVolumeListTabular returns a tabular summary of volume instances.
func formatVolumeListTabular(value interface{}) ([]byte, error) {
	infos, ok := value.(map[string]map[string]VolumeInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", infos, value)
	}
	return formatVolumeListTabularTyped(infos), nil
}

func formatVolumeListTabularTyped(infos map[string]map[string]VolumeInfo) []byte {
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
	print("MACHINE", "DEVICE_NAME", "VOLUME", "SIZE")

	// sort by machines
	machines := make([]string, len(infos))
	i := 0
	for machine := range infos {
		machines[i] = machine
		i++
	}
	sort.Strings(machines)
	for _, machine := range machines {
		machineVolumes := infos[machine]

		// sort by volume
		volumes := make([]string, len(machineVolumes))
		i := 0
		for volume := range machineVolumes {
			volumes[i] = volume
			i++
		}
		sort.Strings(volumes)

		for _, volume := range volumes {
			info := machineVolumes[volume]
			size := humanize.IBytes(info.Size * humanize.MiByte)
			print(machine, info.DeviceName, volume, size)
		}
	}
	tw.Flush()

	return out.Bytes()
}
