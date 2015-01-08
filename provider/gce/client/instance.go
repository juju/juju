// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"strings"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

const (
	partialMachineType = "zones/%s/machineTypes/%s"
)

type InstanceSpec struct {
	ID                string
	Type              string
	Disks             []DiskSpec
	Network           NetworkSpec
	NetworkInterfaces []string
	Metadata          map[string]string
	Tags              []string
}

func (is InstanceSpec) Create(conn *Connection, zones []string) (*Instance, error) {
	raw, err := is.create(conn, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := newInstance(raw)
	copied := is
	inst.spec = &copied
	return inst, nil
}

func (is InstanceSpec) create(conn *Connection, zones []string) (*compute.Instance, error) {
	raw := &compute.Instance{
		Name:              is.ID,
		Disks:             is.disks(),
		NetworkInterfaces: is.networkInterfaces(),
		Metadata:          packMetadata(is.Metadata),
		Tags:              &compute.Tags{Items: is.Tags},
		// MachineType is set in the addInstance call.
	}
	err := conn.addInstance(raw, is.Type, zones)
	return raw, errors.Trace(err)
}

func (is InstanceSpec) disks() []*compute.AttachedDisk {
	var result []*compute.AttachedDisk
	for _, spec := range is.Disks {
		result = append(result, spec.newAttached())
	}
	return result
}

func (is InstanceSpec) networkInterfaces() []*compute.NetworkInterface {
	var result []*compute.NetworkInterface
	for _, name := range is.NetworkInterfaces {
		result = append(result, is.Network.newInterface(name))
	}
	return result
}

type Instance struct {
	ID   string
	Zone string
	raw  compute.Instance
	spec *InstanceSpec
}

func newInstance(raw *compute.Instance) *Instance {
	return &Instance{
		ID:   raw.Name,
		Zone: zoneName(raw),
		raw:  *raw,
	}
}

func (gi Instance) RootDiskGB() int64 {
	if gi.spec == nil {
		return 0
	}
	attached := rootDisk(gi.spec)
	// The root disk from a spec will not fail.
	size, _ := diskSizeGB(attached)
	return size
}

func (gi Instance) Status() string {
	return gi.raw.Status
}

func (gi *Instance) Refresh(conn *Connection) error {
	raw, err := conn.instance(gi.Zone, gi.ID)
	if err != nil {
		return errors.Trace(err)
	}

	gi.raw = *raw
	return nil
}

func (gi Instance) Addresses() ([]network.Address, error) {
	var addresses []network.Address

	for _, netif := range gi.raw.NetworkInterfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.Address{
				Value: accessConfig.NatIP,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, address)

		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := network.Address{
			Value: netif.NetworkIP,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func (gi Instance) Metadata() map[string]string {
	return unpackMetadata(gi.raw.Metadata)
}

func filterInstances(instances []Instance, statuses ...string) []Instance {
	var results []Instance
	for _, inst := range instances {
		if !checkInstStatus(inst, statuses...) {
			continue
		}
		results = append(results, inst)
	}
	return results
}

func checkInstStatus(inst Instance, statuses ...string) bool {
	for _, status := range statuses {
		if inst.Status() == status {
			return true
		}
	}
	return false
}

// FormatAuthorizedKeys returns our authorizedKeys with
// the username prepended to it. This is the format that
// GCE uses for its sshKeys metadata.
func FormatAuthorizedKeys(raw, user string) (string, error) {
	var userKeys string
	users := []string{user}
	keys := strings.Split(raw, "\n")
	for _, key := range keys {
		for _, user := range users {
			userKeys += user + ":" + key + "\n"
		}
	}
	return userKeys, nil
}

func packMetadata(data map[string]string) *compute.Metadata {
	var items []*compute.MetadataItems
	for key, value := range data {
		item := compute.MetadataItems{
			Key:   key,
			Value: value,
		}
		items = append(items, &item)
	}
	return &compute.Metadata{Items: items}
}

func unpackMetadata(data *compute.Metadata) map[string]string {
	if data == nil {
		return nil
	}

	result := make(map[string]string)
	for _, item := range data.Items {
		result[item.Key] = item.Value
	}
	return result
}

func resolveMachineType(zone, name string) string {
	return fmt.Sprintf(partialMachineType, zone, name)
}
