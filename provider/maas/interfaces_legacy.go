// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

func (env *maasEnviron) getNodegroupInterfaces(nodegroups []string) map[string][]net.IP {
	nodegroupsObject := env.getMAASClient().GetSubObject("nodegroups")

	nodegroupsInterfacesMap := make(map[string][]net.IP)
	for _, uuid := range nodegroups {
		interfacesObject := nodegroupsObject.GetSubObject(uuid).GetSubObject("interfaces")
		interfacesResult, err := interfacesObject.CallGet("list", nil)
		if err != nil {
			logger.Debugf("cannot list interfaces for nodegroup %v: %v", uuid, err)
			continue
		}
		interfaces, err := interfacesResult.GetArray()
		if err != nil {
			logger.Debugf("cannot get interfaces for nodegroup %v: %v", uuid, err)
			continue
		}
		for _, interfaceResult := range interfaces {
			nic, err := interfaceResult.GetMap()
			if err != nil {
				logger.Debugf("cannot get interface %v for nodegroup %v: %v", nic, uuid, err)
				continue
			}
			ip, err := nic["ip"].GetString()
			if err != nil {
				logger.Debugf("cannot get interface IP %v for nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_low, err := nic["static_ip_range_low"].GetString()
			if err != nil {
				logger.Debugf("cannot get static IP range lower bound for interface %v on nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_high, err := nic["static_ip_range_high"].GetString()
			if err != nil {
				logger.Infof("cannot get static IP range higher bound for interface %v on nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_low_ip := net.ParseIP(static_low)
			static_high_ip := net.ParseIP(static_high)
			if static_low_ip == nil || static_high_ip == nil {
				logger.Debugf("invalid IP in static range for interface %v on nodegroup %v: %q %q", nic, uuid, static_low_ip, static_high_ip)
				continue
			}
			nodegroupsInterfacesMap[ip] = []net.IP{static_low_ip, static_high_ip}
		}
	}
	return nodegroupsInterfacesMap
}

// setupNetworks prepares a []network.InterfaceInfo for the given
// instance. Any networks in networksToDisable will be configured as
// disabled on the machine. Any disabled network interfaces (as
// discovered from the lshw output for the node) will stay disabled.
// The interface name discovered as primary is also returned.
func (environ *maasEnviron) setupNetworks(inst instance.Instance, networksToDisable set.Strings) ([]network.InterfaceInfo, string, error) {
	// Get the instance network interfaces first.
	interfaces, primaryIface, err := environ.getInstanceNetworkInterfaces(inst)
	if err != nil {
		return nil, "", errors.Annotatef(err, "getInstanceNetworkInterfaces failed")
	}
	logger.Debugf("node %q has network interfaces %v", inst.Id(), interfaces)
	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, "", errors.Annotatef(err, "getInstanceNetworks failed")
	}
	logger.Debugf("node %q has networks %v", inst.Id(), networks)
	var tempInterfaceInfo []network.InterfaceInfo
	for _, netw := range networks {
		disabled := networksToDisable.Contains(netw.Name)
		netCIDR := &net.IPNet{
			IP:   net.ParseIP(netw.IP),
			Mask: net.IPMask(net.ParseIP(netw.Mask)),
		}
		macs, err := environ.getNetworkMACs(netw.Name)
		if err != nil {
			return nil, "", errors.Annotatef(err, "getNetworkMACs failed")
		}
		logger.Debugf("network %q has MACs: %v", netw.Name, macs)
		for _, mac := range macs {
			if ifinfo, ok := interfaces[mac]; ok {
				tempInterfaceInfo = append(tempInterfaceInfo, network.InterfaceInfo{
					MACAddress:    mac,
					InterfaceName: ifinfo.InterfaceName,
					DeviceIndex:   ifinfo.DeviceIndex,
					CIDR:          netCIDR.String(),
					VLANTag:       netw.VLANTag,
					ProviderId:    network.Id(netw.Name),
					NetworkName:   netw.Name,
					Disabled:      disabled || ifinfo.Disabled,
				})
			}
		}
	}
	// Verify we filled-in everything for all networks/interfaces
	// and drop incomplete records.
	var interfaceInfo []network.InterfaceInfo
	for _, info := range tempInterfaceInfo {
		if info.ProviderId == "" || info.NetworkName == "" || info.CIDR == "" {
			logger.Infof("ignoring interface %q: missing subnet info", info.InterfaceName)
			continue
		}
		if info.MACAddress == "" || info.InterfaceName == "" {
			logger.Infof("ignoring subnet %q: missing interface info", info.ProviderId)
			continue
		}
		interfaceInfo = append(interfaceInfo, info)
	}
	logger.Debugf("node %q network information: %#v", inst.Id(), interfaceInfo)
	return interfaceInfo, primaryIface, nil
}

// legacyNetworkInterfaces implements Environ.NetworkInterfaces with pre-1.9
// MAAS API.
func (environ *maasEnviron) legacyNetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	instances, err := environ.acquiredInstances([]instance.Id{instId})
	if err != nil {
		return nil, errors.Annotatef(err, "could not find instance %q", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %q", instId)
	}
	inst := instances[0]
	interfaces, _, err := environ.getInstanceNetworkInterfaces(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get instance %q network interfaces", instId)
	}

	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get instance %q subnets", instId)
	}

	macToNetworksMap := make(map[string][]networkDetails)
	for _, network := range networks {
		macs, err := environ.listConnectedMacs(network)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, mac := range macs {
			if networks, found := macToNetworksMap[mac]; found {
				macToNetworksMap[mac] = append(networks, network)
			} else {
				macToNetworksMap[mac] = append([]networkDetails(nil), network)
			}
		}
	}

	result := []network.InterfaceInfo{}
	for serial, iface := range interfaces {
		deviceIndex := iface.DeviceIndex
		interfaceName := iface.InterfaceName
		disabled := iface.Disabled

		ifaceInfo := network.InterfaceInfo{
			DeviceIndex:   deviceIndex,
			InterfaceName: interfaceName,
			Disabled:      disabled,
			NoAutoStart:   disabled,
			MACAddress:    serial,
			ConfigType:    network.ConfigDHCP,
		}
		allDetails, ok := macToNetworksMap[serial]
		if !ok {
			logger.Debugf("no subnet information for MAC address %q, instance %q", serial, instId)
			continue
		}
		for _, details := range allDetails {
			ifaceInfo.VLANTag = details.VLANTag
			ifaceInfo.ProviderSubnetId = network.Id(details.Name)
			mask := net.IPMask(net.ParseIP(details.Mask))
			cidr := net.IPNet{net.ParseIP(details.IP), mask}
			ifaceInfo.CIDR = cidr.String()
			ifaceInfo.Address = network.NewAddress(cidr.IP.String())
			result = append(result, ifaceInfo)
		}
	}
	return result, nil
}

// listConnectedMacs calls the MAAS list_connected_macs API to fetch all the
// the MAC addresses attached to a specific network.
func (environ *maasEnviron) listConnectedMacs(network networkDetails) ([]string, error) {
	client := environ.getMAASClient().GetSubObject("networks").GetSubObject(network.Name)
	json, err := client.CallGet("list_connected_macs", nil)
	if err != nil {
		return nil, err
	}

	macs, err := json.GetArray()
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, macObj := range macs {
		macMap, err := macObj.GetMap()
		if err != nil {
			return nil, err
		}
		mac, err := macMap["mac_address"].GetString()
		if err != nil {
			return nil, err
		}

		result = append(result, mac)
	}
	return result, nil
}

// networkDetails holds information about a MAAS network.
type networkDetails struct {
	Name        string
	IP          string
	Mask        string
	VLANTag     int
	Description string
}

// getInstanceNetworks returns a list of all MAAS networks for a given node.
func (environ *maasEnviron) getInstanceNetworks(inst instance.Instance) ([]networkDetails, error) {
	nodeId, err := environ.nodeIdFromInstance(inst)
	if err != nil {
		return nil, err
	}
	client := environ.getMAASClient().GetSubObject("networks")
	params := url.Values{"node": {nodeId}}
	json, err := client.CallGet("", params)
	if err != nil {
		return nil, err
	}
	jsonNets, err := json.GetArray()
	if err != nil {
		return nil, err
	}

	networks := make([]networkDetails, len(jsonNets))
	for i, jsonNet := range jsonNets {
		fields, err := jsonNet.GetMap()
		if err != nil {
			return nil, err
		}
		name, err := fields["name"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get name: %v", err)
		}
		ip, err := fields["ip"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get ip: %v", err)
		}
		netmask, err := fields["netmask"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get netmask: %v", err)
		}
		vlanTag := 0
		vlanTagField, ok := fields["vlan_tag"]
		if ok && !vlanTagField.IsNil() {
			// vlan_tag is optional, so assume it's 0 when missing or nil.
			vlanTagFloat, err := vlanTagField.GetFloat64()
			if err != nil {
				return nil, fmt.Errorf("cannot get vlan_tag: %v", err)
			}
			vlanTag = int(vlanTagFloat)
		}
		description, err := fields["description"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get description: %v", err)
		}

		networks[i] = networkDetails{
			Name:        name,
			IP:          ip,
			Mask:        netmask,
			VLANTag:     vlanTag,
			Description: description,
		}
	}
	return networks, nil
}

// getNetworkMACs returns all MAC addresses connected to the given
// network.
func (environ *maasEnviron) getNetworkMACs(networkName string) ([]string, error) {
	client := environ.getMAASClient().GetSubObject("networks").GetSubObject(networkName)
	json, err := client.CallGet("list_connected_macs", nil)
	if err != nil {
		return nil, err
	}
	jsonMACs, err := json.GetArray()
	if err != nil {
		return nil, err
	}

	macs := make([]string, len(jsonMACs))
	for i, jsonMAC := range jsonMACs {
		fields, err := jsonMAC.GetMap()
		if err != nil {
			return nil, err
		}
		macAddress, err := fields["mac_address"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get mac_address: %v", err)
		}
		macs[i] = macAddress
	}
	return macs, nil
}

// getInstanceNetworkInterfaces returns a map of interface MAC address
// to ifaceInfo for each network interface of the given instance, as
// discovered during the commissioning phase. In addition, it also
// returns the interface name discovered as primary.
func (environ *maasEnviron) getInstanceNetworkInterfaces(inst instance.Instance) (map[string]ifaceInfo, string, error) {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	result, err := maasObj.CallGet("details", nil)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	// Get the node's lldp / lshw details discovered at commissioning.
	data, err := result.GetBytes()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	var parsed map[string]interface{}
	if err := bson.Unmarshal(data, &parsed); err != nil {
		return nil, "", errors.Trace(err)
	}
	lshwData, ok := parsed["lshw"]
	if !ok {
		return nil, "", errors.Errorf("no hardware information available for node %q", inst.Id())
	}
	lshwXML, ok := lshwData.([]byte)
	if !ok {
		return nil, "", errors.Errorf("invalid hardware information for node %q", inst.Id())
	}
	// Now we have the lshw XML data, parse it to extract and return NICs.
	return extractInterfaces(inst, lshwXML)
}

type ifaceInfo struct {
	DeviceIndex   int
	InterfaceName string
	Disabled      bool
}

// extractInterfaces parses the XML output of lswh and extracts all
// network interfaces, returing a map MAC address to ifaceInfo, as
// well as the interface name discovered as primary.
func extractInterfaces(inst instance.Instance, lshwXML []byte) (map[string]ifaceInfo, string, error) {
	type Node struct {
		Id          string `xml:"id,attr"`
		Disabled    bool   `xml:"disabled,attr,omitempty"`
		Description string `xml:"description"`
		Serial      string `xml:"serial"`
		LogicalName string `xml:"logicalname"`
		Children    []Node `xml:"node"`
	}
	type List struct {
		Nodes []Node `xml:"node"`
	}
	var lshw List
	if err := xml.Unmarshal(lshwXML, &lshw); err != nil {
		return nil, "", errors.Annotatef(err, "cannot parse lshw XML details for node %q", inst.Id())
	}
	primaryIface := ""
	interfaces := make(map[string]ifaceInfo)
	var processNodes func(nodes []Node) error
	var baseIndex int
	processNodes = func(nodes []Node) error {
		for _, node := range nodes {
			if strings.HasPrefix(node.Id, "network") {
				index := baseIndex
				if strings.HasPrefix(node.Id, "network:") {
					// There is an index suffix, parse it.
					var err error
					index, err = strconv.Atoi(strings.TrimPrefix(node.Id, "network:"))
					if err != nil {
						return errors.Annotatef(err, "lshw output for node %q has invalid ID suffix for %q", inst.Id(), node.Id)
					}
				} else {
					baseIndex++
				}

				if primaryIface == "" && !node.Disabled {
					primaryIface = node.LogicalName
					logger.Debugf("node %q primary network interface is %q", inst.Id(), primaryIface)
				}
				if node.Disabled {
					logger.Debugf("node %q skipping disabled network interface %q", inst.Id(), node.LogicalName)
				}
				interfaces[node.Serial] = ifaceInfo{
					DeviceIndex:   index,
					InterfaceName: node.LogicalName,
					Disabled:      node.Disabled,
				}
			}
			if err := processNodes(node.Children); err != nil {
				return err
			}
		}
		return nil
	}
	err := processNodes(lshw.Nodes)
	return interfaces, primaryIface, err
}
