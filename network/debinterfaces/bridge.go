// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"fmt"
	"strings"
)

type deviceNameSet map[string]bool

// The set of options which should be moved from an existing 'iface'
// stanza when turning it into a bridged device.
var bridgeOnlyOptions = []string{
	"address",
	"gateway",
	"netmask",
	"dns-nameservers",
	"dns-search",
	"dns-sortlist",
}

func pruneOptions(options []string, names ...string) []string {
	toPrune := map[string]bool{}
	for _, v := range names {
		toPrune[v] = true
	}
	result := make([]string, 0, len(options))
	for _, o := range options {
		words := strings.Fields(o)
		if len(words) >= 1 && !toPrune[words[0]] {
			result = append(result, o)
		}
	}
	return result
}

func pruneOptionsWithPrefix(options []string, prefix string) []string {
	result := make([]string, 0, len(options))
	for _, o := range options {
		words := strings.Fields(o)
		if len(words) >= 1 && !strings.HasPrefix(words[0], prefix) {
			result = append(result, o)
		}
	}
	return result
}

func isLoopbackDevice(s *IfaceStanza) bool {
	words := strings.Fields(s.Definition()[0])
	return len(words) >= 4 && words[3] == "loopback"
}

func newAutoStanza(deviceNames ...string) *AutoStanza {
	return &AutoStanza{
		stanza: stanza{
			definition: fmt.Sprintf("auto %s", strings.Join(deviceNames, " ")),
		},
		DeviceNames: deviceNames,
	}
}

func isDefinitionBridgeable(iface *IfaceStanza) bool {
	definitions := iface.definition
	words := strings.Fields(definitions)
	return len(words) >= 4
}

func turnManual(bridgeName string, iface IfaceStanza) *IfaceStanza {
	if iface.IsAlias {
		words := strings.Fields(iface.definition)
		words[1] = bridgeName
		iface.definition = strings.Join(words, " ")
		return &iface
	}
	words := strings.Fields(iface.definition)
	words[3] = "manual"
	iface.definition = strings.Join(words, " ")
	iface.Options = pruneOptions(iface.Options, bridgeOnlyOptions...)
	return &iface
}

func bridgeInterface(bridgeName string, iface IfaceStanza) *IfaceStanza {
	words := strings.Fields(iface.definition)
	words[1] = bridgeName
	iface.definition = strings.Join(words, " ")
	iface.Options = pruneOptions(iface.Options, "mtu")
	if iface.IsVLAN {
		iface.Options = pruneOptions(iface.Options, "vlan_id", "vlan-raw-device")
	}
	if iface.HasBondOptions {
		iface.Options = pruneOptionsWithPrefix(iface.Options, "bond-")
	}
	iface.Options = append(iface.Options, fmt.Sprintf("bridge_ports %s", iface.DeviceName))
	return &iface
}

func isBridgeable(iface *IfaceStanza) bool {
	return isDefinitionBridgeable(iface) &&
		!isLoopbackDevice(iface) &&
		!iface.IsBridged &&
		!iface.HasBondMasterOption
}

// Bridge turns existing devices into bridged devices.
func Bridge(stanzas []Stanza, devices map[string]string) []Stanza {
	result := make([]Stanza, 0)
	autoStanzaSet := map[string]bool{}
	manualInetSet := map[string]bool{}
	manualInet6Set := map[string]bool{}
	bridges := map[string]bool{}
	ifacesToBridge := make([]IfaceStanza, 0)
	devicesToBridge := deviceNameSet{}

	for name := range devices {
		devicesToBridge[name] = true
	}

	// We don't want to perturb the existing order, except for
	// aliases which we do mutate in situ. All the bridged stanzas
	// are added to the end of the original input.

	for _, s := range stanzas {
		switch v := s.(type) {
		case IfaceStanza:
			if devicesToBridge[v.DeviceName] && isBridgeable(&v) {
				// we need to have only iface XXX inet manual
				// TODO do we need to have separate manual for inet6 or is one sufficient?
				if strings.Fields(v.definition)[2] == "inet" && !manualInetSet[v.DeviceName] {
					result = append(result, *turnManual(devices[v.DeviceName], v))
					manualInetSet[v.DeviceName] = true
				}
				if strings.Fields(v.definition)[2] == "inet6" && !manualInet6Set[v.DeviceName] {
					result = append(result, *turnManual(devices[v.DeviceName], v))
					manualInet6Set[v.DeviceName] = true
				}
				ifacesToBridge = append(ifacesToBridge, v)
			} else {
				result = append(result, s)
				if v.IsBridged {
					bridges[v.DeviceName] = true
				}
			}
		case AutoStanza:
			names := v.DeviceNames
			for i, name := range names {
				if isAlias(name) && devicesToBridge[name] {
					names[i] = devices[name]
					autoStanzaSet[names[i]] = true
				} else {
					autoStanzaSet[names[i]] = true
				}
			}
			result = append(result, *newAutoStanza(names...))
		case SourceStanza:
			v.Stanzas = Bridge(v.Stanzas, devices)
			result = append(result, v)
		case SourceDirectoryStanza:
			v.Stanzas = Bridge(v.Stanzas, devices)
			result = append(result, v)
		default:
			result = append(result, v)
		}
	}

	for _, iface := range ifacesToBridge {
		bridgeName := devices[iface.DeviceName]
		// skip it if we already have a bridge of this name
		if _, ok := bridges[bridgeName]; ok {
			continue
		}

		// If there was a `auto $DEVICE` stanza make sure we
		// create the complementary auto stanza for the bridge
		// device.
		if autoStanzaSet[iface.DeviceName] {
			if !autoStanzaSet[bridgeName] {
				autoStanzaSet[bridgeName] = true
				result = append(result, *newAutoStanza(bridgeName))
			}
		}
		if isBridgeable(&iface) && !iface.IsAlias {
			result = append(result, *bridgeInterface(bridgeName, iface))
		}
	}

	return result
}
