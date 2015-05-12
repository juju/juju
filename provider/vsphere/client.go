// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/govmomi"
	"github.com/juju/govmomi/find"
	"github.com/juju/govmomi/list"
	"github.com/juju/govmomi/object"
	"github.com/juju/govmomi/property"
	"github.com/juju/govmomi/vim25/methods"
	"github.com/juju/govmomi/vim25/mo"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

const (
	metadataKeyIsState   = "juju_is_state_key"
	metadataValueIsState = "juju_is_value_value"
)

type client struct {
	connection *govmomi.Client
	datacenter *object.Datacenter
	finder     *find.Finder
	recurser   *list.Recurser
}

var newClient = func(ecfg *environConfig) (*client, error) {
	url, err := ecfg.url()
	if err != nil {
		return nil, err
	}
	connection, err := newConnection(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	finder := find.NewFinder(connection.Client, true)
	datacenter, err := finder.Datacenter(context.TODO(), ecfg.datacenter())
	if err != nil {
		return nil, errors.Trace(err)
	}
	finder.SetDatacenter(datacenter)
	recurser := &list.Recurser{
		Collector: property.DefaultCollector(connection.Client),
		All:       true,
	}
	return &client{
		connection: connection,
		datacenter: datacenter,
		finder:     finder,
		recurser:   recurser,
	}, nil
}

var newConnection = func(url *url.URL) (*govmomi.Client, error) {
	return govmomi.NewClient(context.TODO(), url, true)
}

type instanceSpec struct {
	machineID string
	zone      *vmwareAvailZone
	hwc       *instance.HardwareCharacteristics
	img       *OvaFileMetadata
	userData  []byte
	sshKey    string
	isState   bool
	apiPort   int
}

// CreateInstance create new vm in vsphere and run it
func (c *client) CreateInstance(ecfg *environConfig, spec *instanceSpec) (*mo.VirtualMachine, error) {
	manager := &ovaImportManager{client: c}
	vm, err := manager.importOva(ecfg, spec)
	if err != nil {
		return nil, errors.Annotatef(err, "Failed to import OVA file")
	}
	task, err := vm.PowerOn(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ecfg.externalNetwork() != "" {
		ip, err := vm.WaitForIP(context.TODO())
		if err != nil {
			return nil, errors.Trace(err)
		}
		client := newSshClient(ip)
		err = client.configureExternalIpAddress(spec.apiPort)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	var res mo.VirtualMachine
	err = c.connection.RetrieveOne(context.TODO(), *taskInfo.Entity, nil, &res)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

// RemoveInstances removes vms from the system
func (c *client) RemoveInstances(ids ...string) error {
	var lastError error
	tasks := make([]*object.Task, 0, len(ids))
	for _, id := range ids {
		vm, err := c.finder.VirtualMachine(context.TODO(), id)
		if err != nil {
			lastError = err
			logger.Errorf(err.Error())
			continue
		}
		task, err := vm.PowerOff(context.TODO())
		if err != nil {
			lastError = err
			logger.Errorf(err.Error())
			continue
		}
		tasks = append(tasks, task)
		task, err = vm.Destroy(context.TODO())
		if err != nil {
			lastError = err
			logger.Errorf(err.Error())
			continue
		}
		//We don't wait for task completeon here. Instead we want to run all tasks as soon as posible
		//and then wait for them all. such aproach will run all tasks in parallel
		tasks = append(tasks, task)
	}

	for _, task := range tasks {
		_, err := task.WaitForResult(context.TODO(), nil)
		if err != nil {
			lastError = err
			logger.Errorf(err.Error())
		}
	}
	return errors.Annotatef(lastError, "failed to remowe instances")
}

// Instances return list of all vms in the system, that match naming convention
func (c *client) Instances(prefix string) ([]*mo.VirtualMachine, error) {
	items, err := c.finder.VirtualMachineList(context.TODO(), "*")
	if err != nil {
		return nil, errors.Trace(err)
	}

	var vms []*mo.VirtualMachine
	vms = make([]*mo.VirtualMachine, 0, len(vms))
	for _, item := range items {
		var vm mo.VirtualMachine
		err = c.connection.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if strings.HasPrefix(vm.Name, prefix) {
			vms = append(vms, &vm)
		}
	}

	return vms, nil
}

// Refresh refreshes the virtual machine
func (c *client) Refresh(v *mo.VirtualMachine) error {
	vm, err := c.getVm(v.Name)
	if err != nil {
		return errors.Trace(err)
	}
	*v = *vm
	return nil
}

func (c *client) getVm(name string) (*mo.VirtualMachine, error) {
	item, err := c.finder.VirtualMachine(context.TODO(), name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var vm mo.VirtualMachine
	err = c.connection.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &vm, nil
}

//AvailabilityZones retuns list of all root compute resources in the system
func (c *client) AvailabilityZones() ([]*mo.ComputeResource, error) {
	folders, err := c.datacenter.Folders(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	root := list.Element{
		Object: folders.HostFolder,
	}
	es, err := c.recurser.Recurse(context.TODO(), root, []string{"*"})
	if err != nil {
		return nil, err
	}

	var cprs []*mo.ComputeResource
	for _, e := range es {
		switch o := e.Object.(type) {
		case mo.ClusterComputeResource:
			cprs = append(cprs, &o.ComputeResource)
		case mo.ComputeResource:
			cprs = append(cprs, &o)
		}
	}

	return cprs, nil
}

func (c *client) GetNetworkInterfaces(inst instance.Id, ecfg *environConfig) ([]network.InterfaceInfo, error) {
	vm, err := c.getVm(string(inst))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if vm.Guest == nil {
		return nil, errors.Errorf("vm guest is not initialized")
	}
	res := make([]network.InterfaceInfo, 0)
	for _, net := range vm.Guest.Net {
		ipScope := network.ScopeCloudLocal
		if net.Network == ecfg.externalNetwork() {
			ipScope = network.ScopePublic
		}
		res = append(res, network.InterfaceInfo{
			DeviceIndex:      net.DeviceConfigId,
			MACAddress:       net.MacAddress,
			NetworkName:      net.Network,
			Disabled:         !net.Connected,
			ProviderId:       network.Id(fmt.Sprintf("net-device%d", net.DeviceConfigId)),
			ProviderSubnetId: network.Id(net.Network),
			InterfaceName:    fmt.Sprintf("unsupported%d", net.DeviceConfigId),
			ConfigType:       network.ConfigDHCP,
			Address:          network.NewScopedAddress(net.IpAddress[0], ipScope),
		})
	}
	return res, nil
}

func (c *client) Subnets(inst instance.Id, ids []network.Id) ([]network.SubnetInfo, error) {
	if len(ids) == 0 {
		return nil, errors.Errorf("subnetIds must not be empty")
	}
	vm, err := c.getVm(string(inst))
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]network.SubnetInfo, 0)
	req := &types.QueryIpPools{
		This: *c.connection.ServiceContent.IpPoolManager,
		Dc:   c.datacenter.Reference(),
	}
	ipPools, err := methods.QueryIpPools(context.TODO(), c.connection.Client, req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, vmNet := range vm.Guest.Net {
		existId := false
		for _, id := range ids {
			if string(id) == vmNet.Network {
				existId = true
				break
			}
		}
		if !existId {
			continue
		}
		var netPool *types.IpPool
		for _, pool := range ipPools.Returnval {
			for _, association := range pool.NetworkAssociation {
				if association.NetworkName == vmNet.Network {
					netPool = &pool
					break
				}
			}
		}
		subnet := network.SubnetInfo{
			ProviderId: network.Id(vmNet.Network),
		}
		if netPool != nil && netPool.Ipv4Config != nil {
			low, high, err := c.ParseNetworkRange(netPool.Ipv4Config.Range)
			if err != nil {
				logger.Warningf(err.Error())
			} else {
				subnet.AllocatableIPLow = low
				subnet.AllocatableIPHigh = high
			}
		}
		res = append(res)
	}
	return res, nil
}

func (c *client) ParseNetworkRange(netRange string) (net.IP, net.IP, error) {
	//netPool.Range is specified as a set of ranges separated with commas. One range is given by a start address, a hash (#), and the length of the range.
	//For example:
	//192.0.2.235 # 20 is the IPv4 range from 192.0.2.235 to 192.0.2.254
	ranges := strings.Split(netRange, ",")
	if len(ranges) > 0 {
		rangeSplit := strings.Split(ranges[0], "#")
		if len(rangeSplit) == 2 {
			if rangeLen, err := strconv.ParseInt(rangeSplit[1], 10, 8); err == nil {
				ipSplit := strings.Split(rangeSplit[0], ".")
				if len(ipSplit) == 4 {
					if lastSegment, err := strconv.ParseInt(ipSplit[3], 10, 8); err != nil {
						lastSegment += rangeLen - 1
						if lastSegment > 254 {
							lastSegment = 254
						}
						return net.ParseIP(rangeSplit[0]), net.ParseIP(fmt.Sprintf("%s.%s.%s.%d", ipSplit[0], ipSplit[1], ipSplit[2], lastSegment)), nil
					}
				}
			}
		}
	}
	return nil, nil, errors.Errorf("can't parse netRange: %s", netRange)
}
