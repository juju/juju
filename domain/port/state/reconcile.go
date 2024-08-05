// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/interval"
	"github.com/juju/juju/core/network"
)

func reconcilePorts(currentOpened, openPorts, closePorts []network.PortRange) []network.PortRange {
	reconciled := map[string]interval.IntegerIntervals{
		"tcp": interval.NewIntegerIntervals(),
		"udp": interval.NewIntegerIntervals(),
	}
	var icmp bool

	for _, port := range append(currentOpened, openPorts...) {
		if port.Protocol == "icmp" {
			icmp = true
			continue
		}
		reconciled[port.Protocol] = reconciled[port.Protocol].Union(interval.NewIntegerInterval(port.FromPort, port.ToPort))
	}

	for _, port := range closePorts {
		if port.Protocol == "icmp" {
			icmp = false
			continue
		}
		reconciled[port.Protocol] = reconciled[port.Protocol].Difference(interval.NewIntegerInterval(port.FromPort, port.ToPort))
	}

	reconciledPorts := []network.PortRange{}
	for protocol, intervals := range reconciled {
		if protocol == "icmp" {
			continue
		}
		for _, interval := range intervals {
			reconciledPorts = append(reconciledPorts, network.PortRange{
				Protocol: protocol,
				FromPort: interval.Lower,
				ToPort:   interval.Upper,
			})
		}
	}
	if icmp {
		reconciledPorts = append(reconciledPorts, network.PortRange{
			Protocol: "icmp",
		})
	}

	return reconciledPorts
}
