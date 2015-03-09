package vmware

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/debug"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/instance"
)

const (
	metadataKeyIsState = "juju-is-state"
)

type tracer struct {
	buf *bytes.Buffer
}

func (t *tracer) Write(p []byte) (n int, err error) {
	return t.buf.Write(p)
}

func (t *tracer) Close() error {
	return nil
}

type logProvider struct {
	tracer *tracer
	base   loggo.Logger
}

func (l *logProvider) NewFile(s string) io.WriteCloser {
	return l.tracer
}

func (l *logProvider) Flush() {
	l.base.Tracef(l.tracer.buf.String())
	l.tracer.buf.Reset()
}

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
	var provider = &logProvider{
		base: logger,
		tracer: &tracer{
			buf: &bytes.Buffer{},
		},
	}
	debug.SetProvider(provider)
	connection, err := govmomi.NewClient(*url, true)
	if err != nil {
		return nil, errors.Annotate(err, "Error while creating new client")
	}

	finder := find.NewFinder(connection, true)
	datacenter, err := finder.Datacenter(ecfg.datacenter())
	if err != nil {
		return nil, errors.Annotate(err, "Error while searching for datacenter")
	}
	finder.SetDatacenter(datacenter)
	datastore, err := finder.Datastore(ecfg.datastore())
	if err != nil {
		return nil, errors.Annotate(err, "Error while searching for datastore")
	}
	resourcePool, err := finder.ResourcePool(ecfg.resourcePool())
	if err != nil {
		return nil, errors.Annotate(err, "Error while searching for resourcePool")
	}
	return &client{
		connection:   connection,
		datacenter:   datacenter,
		datastore:    datastore,
		resourcePool: resourcePool,
		finder:       finder,
	}, nil
}

type ovfFileItem struct {
	url  *url.URL
	item types.OvfFileItem
	ch   chan progress.Report
}

func (o ovfFileItem) Sink() chan<- progress.Report {
	return o.ch
}

func (c *client) CreateInstance(machineID string, hwc *instance.HardwareCharacteristics, img *OvfFileMetadata, userData []byte, sshKey string) (*mo.VirtualMachine, error) {
	folders, err := c.datacenter.Folders()
	if err != nil {
		return nil, err
	}

	ovf, err := c.downloadOvf(img)
	if err != nil {
		return nil, err
	}

	data, err := utils.Gunzip(userData)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cisp := types.OvfCreateImportSpecParams{
		EntityName: machineID,
		PropertyMapping: []types.KeyValue{
			types.KeyValue{Key: "public-keys", Value: sshKey},
			types.KeyValue{Key: "user-data", Value: base64.StdEncoding.EncodeToString(data)},
		},
	}

	spec, err := c.connection.OvfManager().CreateImportSpec(string(ovf), c.resourcePool, c.datastore, cisp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec
	s.NumCPUs = int(*hwc.CpuCores)
	s.MemoryMB = int64(*hwc.Mem)
	s.CpuAllocation = &types.ResourceAllocationInfo{
		Limit:       int64(*hwc.CpuPower),
		Reservation: int64(*hwc.CpuPower),
	}
	for _, d := range s.DeviceChange {
		if disk, ok := d.GetVirtualDeviceConfigSpec().Device.(*types.VirtualDisk); ok {
			disk.CapacityInKB = int64(*hwc.RootDisk) * 1024 * 1024
		}
		n := &d.GetVirtualDeviceConfigSpec().Device.GetVirtualDevice().UnitNumber
		if *n == 0 {
			*n = -1
		}
	}

	lease, err := c.resourcePool.ImportVApp(spec.ImportSpec, folders.VmFolder, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info, err := lease.Wait()
	if err != nil {
		return nil, errors.Trace(err)
	}
	items := []ovfFileItem{}
	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}

			u, err := c.connection.Client.ParseURL(device.Url)
			if err != nil {
				return nil, errors.Trace(err)
			}

			i := ovfFileItem{
				url:  u,
				item: item,
				ch:   make(chan progress.Report),
			}
			items = append(items, i)
		}
	}

	for _, i := range items {
		ind := strings.LastIndex(img.Url, "/")
		err = c.upload(i, img.Url[:ind])
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lease.HttpNfcLeaseComplete()
	vm := govmomi.NewVirtualMachine(c.connection, info.Entity)
	task, err := vm.PowerOn()
	if err != nil {
		return nil, err
	}
	taskInfo, err := task.WaitForResult(nil)
	if err != nil {
		return nil, err
	}
	var res mo.VirtualMachine
	err = c.connection.Properties(*taskInfo.Entity, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *client) upload(ofi ovfFileItem, baseUrl string) error {

	f, err := c.downloadImg(strings.Join([]string{baseUrl, ofi.item.Path}, "/"))
	if err != nil {
		return err
	}

	defer f.Close()

	opts := soap.Upload{
		ContentLength: ofi.item.Size,
		Progress:      ofi,
	}

	opts.Method = "POST"
	opts.Type = "application/x-vnd.vmware-streamVmdk"
	logger.Debugf("Uploading image to %s, size: %d", ofi.url, ofi.item.Size)
	go func() {
		//var pr progress.Report
		for {
			<-ofi.ch
			//logger.Debugf("Progress: %s", pr.Percentage())
		}
	}()
	err = c.connection.Client.Upload(f, ofi.url, &opts)
	if err == nil {
		logger.Debugf("Image uploaded")
	}
	return err
}

func (c *client) downloadOvf(img *OvfFileMetadata) (string, error) {
	resp, err := http.Get(img.Url)
	if err != nil {
		return "", errors.Trace(err)
	}
	if resp.StatusCode != 200 {
		return "", errors.Errorf("Can't download ovf file from url: %s, status: %s", img.Url, resp.StatusCode)
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(bytes), nil
}

func (c *client) downloadImg(url string) (io.ReadCloser, error) {
	logger.Debugf("Downloading image from url %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.StatusCode != 200 {
		return nil, errors.Errorf("Can't download vmdk file from url: %s, status: %s", url, resp.StatusCode)
	}

	return resp.Body, nil
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
