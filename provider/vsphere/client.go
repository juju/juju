// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/list"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

const (
	metadataKeyIsController     = "juju_is_controller_key"
	metadataValueIsController   = "juju_is_controller_value"
	metadataKeyControllerUUID   = "juju_controller_uuid_key"
	metadataValueControllerUUID = "juju_controller_uuid_value"
)

type client struct {
	connectionURL *url.URL
	cloud         environs.CloudSpec
}

var newConnection = func(url *url.URL) (*govmomi.Client, func() error, error) {
	conn, err := govmomi.NewClient(context.TODO(), url, true)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	logout := func() error {
		return conn.Logout(context.TODO())
	}
	return conn, logout, nil

}

func (c *client) connection() (*govmomi.Client, func() error, error) {
	return newConnection(c.connectionURL)
}

func (c *client) recurser(conn *govmomi.Client) *list.Recurser {
	return &list.Recurser{
		Collector: property.DefaultCollector(conn.Client),
		All:       true,
	}
}

func (c *client) finder(conn *govmomi.Client) (*find.Finder, *object.Datacenter, error) {
	finder := find.NewFinder(conn.Client, true)
	datacenter, err := finder.Datacenter(context.TODO(), c.cloud.Region)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	finder.SetDatacenter(datacenter)
	return finder, datacenter, nil
}

var newClient = func(cloudSpec environs.CloudSpec) (*client, error) {

	credAttrs := cloudSpec.Credential.Attributes()
	username := credAttrs[credAttrUser]
	password := credAttrs[credAttrPassword]
	connURL := &url.URL{
		Scheme: "https",
		User:   url.UserPassword(username, password),
		Host:   cloudSpec.Endpoint,
		Path:   "/sdk",
	}

	return &client{
		connectionURL: connURL,
		cloud:         cloudSpec,
	}, nil
}

type instanceSpec struct {
	machineID      string
	zone           *vmwareAvailZone
	hwc            *instance.HardwareCharacteristics
	img            *OvaFileMetadata
	userData       []byte
	sshKey         string
	isController   bool
	controllerUUID string
	apiPort        int
}

// CreateInstance create new vm in vsphere and run it
func (c *client) CreateInstance(ecfg *environConfig, spec *instanceSpec) (*mo.VirtualMachine, error) {
	conn, closer, err := c.connection()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()

	manager := &ovaImportManager{
		client:         conn,
		providerClient: c,
	}
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
	// We assign public ip address for all instances.
	// We can't assign public ip only when OpenPort is called, as assigning
	// an ip address via reconfiguring the VM makes it inaccessible to the
	// controller.
	if ecfg.externalNetwork() != "" {
		ip, err := vm.WaitForIP(context.TODO())
		if err != nil {
			return nil, errors.Trace(err)
		}
		client := common.NewSshInstanceConfigurator(ip)
		err = client.ConfigureExternalIpAddress(spec.apiPort)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	var res mo.VirtualMachine

	err = conn.RetrieveOne(context.TODO(), *taskInfo.Entity, nil, &res)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

// RemoveInstances removes vms from the system
func (c *client) RemoveInstances(ids ...string) error {
	var lastError error
	tasks := make([]*object.Task, 0, len(ids))
	conn, closer, err := c.connection()
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	finder, _, err := c.finder(conn)
	if err != nil {
		return errors.Trace(err)
	}

	for _, id := range ids {
		vm, err := finder.VirtualMachine(context.TODO(), id)
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
	conn, closer, err := c.connection()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	finder, _, err := c.finder(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	items, err := finder.VirtualMachineList(context.TODO(), "*")
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			return nil, nil
		}
		return nil, errors.Annotate(err, "listing VMs")
	}

	var vms []*mo.VirtualMachine
	vms = make([]*mo.VirtualMachine, 0, len(vms))

	for _, item := range items {
		var vm mo.VirtualMachine

		err = conn.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
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
	vm, err := c.vm(v.Name)
	if err != nil {
		return errors.Trace(err)
	}
	*v = *vm
	return nil
}

func (c *client) vm(name string) (*mo.VirtualMachine, error) {
	conn, closer, err := c.connection()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	finder, _, err := c.finder(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	item, err := finder.VirtualMachine(context.TODO(), name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var vm mo.VirtualMachine

	err = conn.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &vm, nil
}

//AvailabilityZones retuns list of all root compute resources in the system
func (c *client) AvailabilityZones() ([]*mo.ComputeResource, error) {
	conn, closer, err := c.connection()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	_, datacenter, err := c.finder(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	folders, err := datacenter.Folders(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	root := list.Element{
		Object: folders.HostFolder,
	}

	es, err := c.recurser(conn).Recurse(context.TODO(), root, []string{"*"})
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
	vm, err := c.vm(string(inst))
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
			DeviceIndex:      int(net.DeviceConfigId),
			MACAddress:       net.MacAddress,
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
	vm, err := c.vm(string(inst))
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]network.SubnetInfo, 0)
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
		res = append(res, network.SubnetInfo{
			ProviderId: network.Id(vmNet.Network),
		})
	}
	return res, nil
}
