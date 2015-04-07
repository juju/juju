package vmware

import (
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

	"github.com/juju/juju/instance"
)

const (
	metadataKeyIsState = "juju-is-state"
)

type client struct {
	connection *govmomi.Client
	datacenter *object.Datacenter
	datastore  *object.Datastore
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
	datastore, err := finder.Datastore(context.TODO(), ecfg.datastore())
	if err != nil {
		return nil, errors.Trace(err)
	}
	recurser := &list.Recurser{
		Collector: property.DefaultCollector(connection.Client),
	}
	return &client{
		connection: connection,
		datacenter: datacenter,
		datastore:  datastore,
		finder:     finder,
		recurser:   recurser,
	}, nil
}

var newConnection = func(url *url.URL) (*govmomi.Client, error) {
	return govmomi.NewClient(context.TODO(), url, true)
}

func (c *client) CreateInstance(machineID string, zone *vmwareAvailZone, hwc *instance.HardwareCharacteristics, img *OvfFileMetadata, userData []byte, sshKey string, isState bool) (*mo.VirtualMachine, error) {
	manager := &ovfImportManager{client: c}
	vm, err := manager.importOvf(machineID, zone, hwc, img, userData, sshKey, isState)
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
