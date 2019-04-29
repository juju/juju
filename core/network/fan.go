// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
)

// FanConfigEntry defines a configuration for single fan.
type FanConfigEntry struct {
	Underlay *net.IPNet
	Overlay  *net.IPNet
}

// FanConfig defines a set of fan configurations for the model.
type FanConfig []FanConfigEntry

// ParseFanConfig parses fan configuration from model-config in the format:
// "underlay1=overlay1 underlay2=overlay2" eg. "172.16.0.0/16=253.0.0.0/8 10.0.0.0/12:254.0.0.0/7"
func ParseFanConfig(line string) (config FanConfig, err error) {
	if line == "" {
		return nil, nil
	}
	entries := strings.Split(line, " ")
	config = make([]FanConfigEntry, len(entries))
	for i, line := range entries {
		cidrs := strings.Split(line, "=")
		if len(cidrs) != 2 {
			return nil, fmt.Errorf("invalid FAN config entry: %v", line)
		}
		if _, config[i].Underlay, err = net.ParseCIDR(strings.TrimSpace(cidrs[0])); err != nil {
			return nil, errors.Annotatef(err, "invalid address in FAN config")
		}
		if _, config[i].Overlay, err = net.ParseCIDR(strings.TrimSpace(cidrs[1])); err != nil {
			return nil, errors.Annotatef(err, "invalid address in FAN config")
		}
		underlaySize, _ := config[i].Underlay.Mask.Size()
		overlaySize, _ := config[i].Overlay.Mask.Size()
		if underlaySize <= overlaySize {
			return nil, fmt.Errorf("invalid FAN config, underlay mask must be larger than overlay: %s", line)
		}
	}
	return config, nil
}

func (fc *FanConfig) String() (line string) {
	configs := make([]string, len(*fc))
	for i, fan := range *fc {
		configs[i] = fmt.Sprintf("%s=%s", fan.Underlay.String(), fan.Overlay.String())
	}
	return strings.Join(configs, " ")
}

// CalculateOverlaySegment takes underlay CIDR and FAN config entry and
// cuts the segment of overlay that corresponds to this underlay:
// eg. for FAN 172.31/16 -> 243/8 and physical subnet 172.31.64/20
// we get FAN subnet 243.64/12.
func CalculateOverlaySegment(underlayCIDR string, fan FanConfigEntry) (*net.IPNet, error) {
	_, underlayNet, err := net.ParseCIDR(underlayCIDR)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetSize, _ := underlayNet.Mask.Size()
	underlaySize, _ := fan.Underlay.Mask.Size()
	if underlaySize <= subnetSize && fan.Underlay.Contains(underlayNet.IP) {
		overlaySize, _ := fan.Overlay.Mask.Size()
		newOverlaySize := overlaySize + (subnetSize - underlaySize)
		fanSize := uint(underlaySize - overlaySize)
		newFanIP := underlayNet.IP.To4()
		if newFanIP == nil {
			return nil, errors.New("fan address is not an IPv4 address.")
		}
		for i := 0; i < 4; i++ {
			newFanIP[i] &^= fan.Underlay.Mask[i]
		}
		numIp := binary.BigEndian.Uint32(newFanIP)
		numIp <<= fanSize
		binary.BigEndian.PutUint32(newFanIP, numIp)
		for i := 0; i < 4; i++ {
			newFanIP[i] += fan.Overlay.IP[i]
		}
		return &net.IPNet{IP: newFanIP, Mask: net.CIDRMask(newOverlaySize, 32)}, nil
	} else {
		return nil, nil
	}
}
