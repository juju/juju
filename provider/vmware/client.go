package vmware

import (
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"

	"github.com/juju/juju/instance"
)

const (
	metadataKeyIsState = "juju-is-state"
)

type client struct {
	connection   *govmomi.Client
	datacenter   *object.Datacenter
	datastore    *object.Datastore
	resourcePool *object.ResourcePool
	finder       *find.Finder
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
	datastore, err := finder.Datastore(context.TODO(), ecfg.datastore())
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourcePool, err := finder.ResourcePool(context.TODO(), ecfg.resourcePool())
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

var newConnection = func(url *url.URL) (*govmomi.Client, error) {
	return govmomi.NewClient(context.TODO(), url, true)
}

func (c *client) CreateInstance(machineID string, hwc *instance.HardwareCharacteristics, img *OvfFileMetadata, userData []byte, sshKey string, isState bool) (*mo.VirtualMachine, error) {
	manager := &ovfImportManager{client: c}
	vm, err := manager.importOvf(machineID, hwc, img, userData, sshKey, isState)
	if err != nil {
		return nil, errors.Annotatef(err, "Failed to import ovf file")
	}
	task, err := vm.PowerOn(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := task.WaitForResult(context.TODO(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res mo.VirtualMachine
	err = c.connection.RetrieveOne(context.TODO(), *taskInfo.Entity, nil, &res)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

func (c *client) RemoveInstances(ids ...string) error {
	var firstError error
	tasks := make([]*object.Task, len(ids))
	for _, id := range ids {
		vm, err := c.finder.VirtualMachine(context.TODO(), id)
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
		task, err := vm.Destroy(context.TODO())
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
		//We don't wait for task completeon here. Instead we want to run all tasks as soon as posible
		//and then wait for them all. such aproach will run all tasks in parallel
		tasks = append(tasks, task)
	}

	for _, task := range tasks {
		_, err := task.WaitForResult(context.TODO(), nil)
		if err != nil && firstError == nil {
			firstError = err
			continue
		}
	}
	return errors.Annotatef(firstError, "Failed while remowing instances")
}

func (c *client) Instances(prefix string) ([]*mo.VirtualMachine, error) {
	items, err := c.finder.VirtualMachineList(context.TODO(), "*")
	if err != nil {
		return nil, errors.Trace(err)
	}

	var vms []*mo.VirtualMachine
	vms = make([]*mo.VirtualMachine, len(vms))
	for _, item := range items {
		var vm mo.VirtualMachine
		err = c.connection.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
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
	item, err := c.finder.VirtualMachine(context.TODO(), v.Config.Name)
	if err != nil {
		return errors.Trace(err)
	}
	var vm mo.VirtualMachine
	err = c.connection.RetrieveOne(context.TODO(), item.Reference(), nil, &vm)
	if err != nil {
		return errors.Trace(err)
	}
	*v = vm
	return nil
}
