package vmware

import (
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/instance"
)

const (
	metadataKeyIsState = "juju-is-state"
)

type client struct {
	connection   *govmomi.Client
	datacenter   *govmomi.Datacenter
	datastore    *govmomi.Datastore
	resourcePool *govmomi.ResourcePool
	finder       *find.Finder
}

func newClient(ecfg *environConfig) (*client, error) {
	url, err := ecfg.url()
	if err != nil {
		return nil, err
	}
	connection, err := govmomi.NewClient(*url, true)
	if err != nil {
		return nil, errors.Trace(err)
	}

	finder := find.NewFinder(connection, true)
	datacenter, err := finder.Datacenter(ecfg.datacenter())
	if err != nil {
		return nil, errors.Trace(err)
	}
	finder.SetDatacenter(datacenter)
	datastore, err := finder.Datastore(ecfg.datastore())
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourcePool, err := finder.ResourcePool(ecfg.resourcePool())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &client{
		connection:   connection,
		datacenter:   datacenter,
		datastore:    datastore,
		resourcePool: resourcePool,
		finder:       finder,
	}, nil
}

func (c *client) CreateInstance(machineID string, hwc *instance.HardwareCharacteristics, img *OvfFileMetadata, userData []byte, sshKey string, isState bool) (*mo.VirtualMachine, error) {

	manager := &ovfImportManager{client: c}
	vm, err := manager.importOvf(machineID, hwc, img, userData, sshKey, isState)
	if err != nil {
		return nil, errors.Annotatef(err, "Failed to import ovf file")
	}
	task, err := vm.PowerOn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := task.WaitForResult(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res mo.VirtualMachine
	err = c.connection.Properties(*taskInfo.Entity, nil, &res)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

func (c *client) RemoveInstances(ids ...string) error {
	var firstError error
	tasks := make([]*govmomi.Task, len(ids))
	for _, id := range ids {
		vm, err := c.finder.VirtualMachine(id)
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
		task, err := vm.Destroy()
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
		//We don't wait for task completeon here. Instead we want to run all tasks as soon as posible
		//and then wait for them all. such aproach will run all tasks in parallel
		tasks = append(tasks, task)
	}

	for _, task := range tasks {
		_, err := task.WaitForResult(nil)
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
	}
	return errors.Annotatef(firstError, "Failed while remowing instances")
}

func (c *client) Instances(prefix string) ([]*mo.VirtualMachine, error) {
	items, err := c.finder.VirtualMachineList("*")
	if err != nil {
		return nil, errors.Trace(err)
	}

	var vms []*mo.VirtualMachine
	vms = make([]*mo.VirtualMachine, len(vms))
	for _, item := range items {
		var vm mo.VirtualMachine
		err = c.connection.Properties(item.Reference(), nil, &vm)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if vm.Config != nil && strings.HasPrefix(vm.Config.Name, prefix) {
			vms = append(vms, &vm)
		}
	}

	return vms, nil
}

func (c *client) Refresh(v *mo.VirtualMachine) error {
	item, err := c.finder.VirtualMachine(v.Config.Name)
	if err != nil {
		return errors.Trace(err)
	}
	var vm mo.VirtualMachine
	err = c.connection.Properties(item.Reference(), nil, &vm)
	if err != nil {
		return errors.Trace(err)
	}
	*v = vm
	return nil
}
