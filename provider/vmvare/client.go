package vmware

import (
	"github.com/juju/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	metadataKeyIsState = "juju-is-state"
)

type client struct {
	connection   *govmomi.Client
	datacenter   *govmomi.Datacenter
	datastore    *govmomi.Datastore
	resourcePool *govmomi.ResourcePool
}

func newClient(ecfg *environConfig) (*client, error) {
	url, err := ecfg.url()
	if err != nil {
		return nil, err
	}
	connection, err := govmomi.NewClient(*url, false)
	if err != nil {
		return nil, err
	}

	finder := find.NewFinder(connection, true)
	datacenter, err := finder.Datacenter(ecfg.datacenter())
	if err != nil {
		return nil, err
	}
	datastore, err := finder.Datastore(ecfg.datastore())
	if err != nil {
		return nil, err
	}
	resourcePool, err := finder.ResourcePool(ecfg.resourcePool())
	if err != nil {
		return nil, err
	}
	return &client{
		connection:   connection,
		datacenter:   datacenter,
		datastore:    datastore,
		resourcePool: resourcePool,
	}, nil
}

func (c *client) CreateInstance(spec types.VirtualMachineConfigSpec, disk string, diskSize int64, userData []byte) (*mo.VirtualMachine, error) {
	folders, err := c.datacenter.Folders()
	if err != nil {
		return nil, err
	}

	task, err := folders.VmFolder.CreateVM(spec, c.resourcePool, nil)
	if err != nil {
		return nil, err
	}

	info, err := task.WaitForResult(nil)
	if err != nil {
		return nil, err
	}

	vm := govmomi.NewVirtualMachine(c.connection, info.Result.(types.ManagedObjectReference))
	devices, err := vm.Device()
	if err != nil {
		return nil, err
	}

	task, err = c.connection.VirtualDiskManager().CopyVirtualDisk(disk, c.datacenter, spec.Name, c.datacenter, nil, false)
	if err != nil {
		return nil, err
	}
	info, err = task.WaitForResult(nil)
	if err != nil {
		return nil, err
	}

	task, err = c.extendVirtualDiskTask(c.datastore.Path(spec.Name), diskSize)
	if err != nil {
		return nil, err
	}
	info, err = task.WaitForResult(nil)
	if err != nil {
		return nil, err
	}

	var add []types.BaseVirtualDevice
	controller, err := devices.FindIDEController("")
	if err != nil {
		return nil, err
	}
	add = append(add, devices.CreateDisk(controller, c.datastore.Path(spec.Name)))

	cdrom, err := devices.CreateCdrom(controller)
	if err != nil {
		return nil, err
	}
	isoPath, err := c.generateUserDataIso(userData)
	if err != nil {
		return nil, err
	}
	add = append(add, devices.InsertIso(cdrom, isoPath))

	err = vm.AddDevice(add...)
	if err != nil {
		return nil, err
	}

	task, err = vm.PowerOn()
	if err != nil {
		return nil, err
	}
	var res mo.VirtualMachine
	err = c.connection.Properties(vm.Reference(), []string{""}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *client) extendVirtualDiskTask(name string, sizeMb int64) (*govmomi.Task, error) {
	datacenter := c.datacenter.Reference()
	req := types.ExtendVirtualDisk_Task{
		This:          *c.connection.ServiceContent.VirtualDiskManager,
		Name:          name,
		Datacenter:    &datacenter,
		NewCapacityKb: sizeMb * 1024,
	}

	res, err := methods.ExtendVirtualDisk_Task(c.connection, &req)
	if err != nil {
		return nil, err
	}

	return govmomi.NewTask(c.connection, res.Returnval), nil
}

func (c *client) generateUserDataIso(userData []byte) (path string, err error) {
	return "", nil
}

func (c *client) RemoveInstances(prefix string, instances ...string) error {
	return errors.NotImplementedf("")
}

func (c *client) Instances(prefix string) ([]mo.VirtualMachine, error) {
	return nil, errors.NotImplementedf("")
}

func (c *client) Refresh(v *mo.VirtualMachine) error {
	return errors.NotImplementedf("")
}
