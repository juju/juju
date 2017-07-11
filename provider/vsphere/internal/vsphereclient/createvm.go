// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/constraints"
	jujuworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

// CreateVirtualMachineParams contains the parameters required for creating
// a new virtual machine.
type CreateVirtualMachineParams struct {
	// Name is the name to give the virtual machine. The VM name is used
	// for its hostname also.
	Name string

	// Folder is the path of the VM folder, relative to the root VM folder,
	// in which to create the VM.
	Folder string

	// OVAContentsDir is the directory containing the extracted OVA contents.
	OVADir string

	// OVF contains the OVF content.
	OVF string

	// UserData is the cloud-init user-data.
	UserData string

	// ComputeResource is the compute resource (host or cluster) to be used
	// to create the VM.
	ComputeResource *mo.ComputeResource

	// Datastore is the name of the datastore in which to create the VM.
	// If this is empty, any accessible datastore will be used.
	Datastore string

	// Metadata are metadata key/value pairs to apply to the VM as
	// "extra config".
	Metadata map[string]string

	// Constraints contains the resource constraints for the virtual machine.
	Constraints constraints.Value

	// ExternalNetwork, if set, is the name of an additional "external"
	// network to which the VM should be connected.
	ExternalNetwork string

	// UpdateProgress is a function that should be called before/during
	// long-running operations to provide a progress reporting.
	UpdateProgress func(string)

	// UpdateProgressInterval is the amount of time to wait between calls
	// to UpdateProgress. This should be lower when the operation is
	// interactive (bootstrap), and higher when non-interactive.
	UpdateProgressInterval time.Duration

	// Clock is used for controlling the timing of progress updates.
	Clock clock.Clock
}

// CreateVirtualMachine creates and powers on a new VM.
//
// This method imports an OVF template using the vSphere API. This process
// comprises the following steps:
//   1. Download the OVA archive, extract it, and load the OVF file contained
//      within. This must have happened before CreateVirtualMachine is called.
//   2. Call CreateImportSpec [0], which validates the OVF descriptor against
//      the hardware supported by the host system. If the validation succeeds,
//      the method returns a result containing:
//        - an ImportSpec to use for importing the entity
//        - a list of items to upload from the OVA (e.g. VMDKs)
//   3. Prepare all necessary parameters (CPU, memory, root disk, etc.), and
//      call the ImportVApp method [0]. This method is responsible for actually
//      creating the VM. An HttpNfcLease [1] object is returned, which is used
//      to signal completion of the process.
//   4. Upload virtual disk contents (usually consists of a single VMDK file)
//   5. Call HttpNfcLeaseComplete [0] to signal completion of uploading,
//      completing the process of creating the virtual machine.
//
// [0] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/
// [1] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.HttpNfcLease.html
func (c *Client) CreateVirtualMachine(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*mo.VirtualMachine, error) {

	args.UpdateProgress("creating import spec")
	spec, err := c.createImportSpec(ctx, args)
	if err != nil {
		return nil, errors.Annotate(err, "creating import spec")
	}

	// Locate the folder in which to create the VM.
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folderPath := path.Join(folders.VmFolder.InventoryPath, args.Folder)
	vmFolder, err := finder.Folder(ctx, folderPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Import the VApp.
	args.UpdateProgress(fmt.Sprintf("creating VM %q", args.Name))
	c.logger.Debugf("creating VM in folder %s", vmFolder)
	rp := object.NewResourcePool(c.client.Client, *args.ComputeResource.ResourcePool)
	lease, err := rp.ImportVApp(ctx, spec.ImportSpec, vmFolder, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to import vapp")
	}

	// Upload the VMDK.
	info, err := lease.Wait(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	type uploadItem struct {
		item types.OvfFileItem
		url  *url.URL
	}
	var uploadItems []uploadItem
	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}
			u, err := c.client.Client.ParseURL(device.Url)
			if err != nil {
				return nil, errors.Trace(err)
			}
			uploadItems = append(uploadItems, uploadItem{
				item: item,
				url:  u,
			})
		}
	}
	leaseUpdaterContext := leaseUpdaterContext{lease: lease}
	for _, item := range uploadItems {
		leaseUpdaterContext.total += item.item.Size
	}
	for _, item := range uploadItems {
		leaseUpdaterContext.size = item.item.Size
		if err := uploadImage(
			ctx,
			c.client.Client,
			item.item,
			args.OVADir,
			item.url,
			args.UpdateProgress,
			args.UpdateProgressInterval,
			leaseUpdaterContext,
			args.Clock,
			c.logger,
		); err != nil {
			return nil, errors.Annotatef(
				err, "uploading %s to %s",
				filepath.Base(item.item.Path),
				item.url,
			)
		}
		leaseUpdaterContext.start += leaseUpdaterContext.size
	}
	if err := lease.HttpNfcLeaseComplete(ctx); err != nil {
		return nil, errors.Trace(err)
	}

	// Finally, power on and return the VM.
	args.UpdateProgress("powering on")
	vm := object.NewVirtualMachine(c.client.Client, info.Entity)
	task, err := vm.PowerOn(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, *taskInfo.Entity, nil, &res); err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

func (c *Client) createImportSpec(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*types.OvfCreateImportSpecResult, error) {
	cisp := types.OvfCreateImportSpecParams{
		EntityName: args.Name,
		PropertyMapping: []types.KeyValue{
			types.KeyValue{Key: "user-data", Value: string(args.UserData)},
			types.KeyValue{Key: "hostname", Value: string(args.Name)},
		},
	}
	ovfManager := object.NewOvfManager(c.client.Client)
	resourcePool := object.NewReference(c.client.Client, *args.ComputeResource.ResourcePool)
	datastore, err := c.selectDatastore(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	spec, err := ovfManager.CreateImportSpec(ctx, args.OVF, resourcePool, datastore, cisp)
	if err != nil {
		return nil, errors.Trace(err)
	} else if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec

	// Apply resource constraints.
	if args.Constraints.HasCpuCores() {
		s.NumCPUs = int32(*args.Constraints.CpuCores)
	}
	if args.Constraints.HasMem() {
		s.MemoryMB = int64(*args.Constraints.Mem)
	}
	if args.Constraints.HasCpuPower() {
		cpuPower := int64(*args.Constraints.CpuPower)
		s.CpuAllocation = &types.ResourceAllocationInfo{
			Limit:       cpuPower,
			Reservation: cpuPower,
		}
	}
	for _, d := range s.DeviceChange {
		disk, ok := d.GetVirtualDeviceConfigSpec().Device.(*types.VirtualDisk)
		if !ok {
			continue
		}
		var rootDisk int64
		if args.Constraints.RootDisk != nil {
			rootDisk = int64(*args.Constraints.RootDisk) * 1024
		}
		if disk.CapacityInKB < rootDisk {
			disk.CapacityInKB = rootDisk
		}
		// Set UnitNumber to -1 if it is unset in ovf file template
		// (in this case it is parses as 0), because 0 causes an error
		// for disk devices.
		var unitNumber int32
		if disk.UnitNumber != nil {
			unitNumber = *disk.UnitNumber
		}
		if unitNumber == 0 {
			unitNumber = -1
			disk.UnitNumber = &unitNumber
		}
	}

	// Apply metadata. Note that we do not have the ability set create or
	// apply tags that will show up in vCenter, as that requires a separate
	// vSphere Automation that we do not have an SDK for.
	for k, v := range args.Metadata {
		s.ExtraConfig = append(s.ExtraConfig, &types.OptionValue{Key: k, Value: v})
	}

	// Add the external network, if any.
	if args.ExternalNetwork != "" {
		s.DeviceChange = append(s.DeviceChange, &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualE1000{
				VirtualEthernetCard: types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Backing: &types.VirtualEthernetCardNetworkBackingInfo{
							VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
								DeviceName: args.ExternalNetwork,
							},
						},
						Connectable: &types.VirtualDeviceConnectInfo{
							StartConnected:    true,
							AllowGuestControl: true,
						},
					},
				},
			},
		})
	}
	return spec, nil
}

func (c *Client) selectDatastore(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*object.Datastore, error) {
	// Select a datastore. If the user specified one, use that; otherwise
	// choose the first one in the list that is accessible.
	refs := make([]types.ManagedObjectReference, len(args.ComputeResource.Datastore))
	for i, ds := range args.ComputeResource.Datastore {
		refs[i] = ds.Reference()
	}
	var datastores []mo.Datastore
	if err := c.client.Retrieve(ctx, refs, nil, &datastores); err != nil {
		return nil, errors.Annotate(err, "retrieving datastore details")
	}
	if args.Datastore != "" {
		for _, ds := range datastores {
			if ds.Name == args.Datastore {
				return object.NewDatastore(c.client.Client, ds.Reference()), nil
			}
		}
		return nil, errors.Errorf("could not find datastore %q", args.Datastore)
	}
	for _, ds := range datastores {
		if ds.Summary.Accessible {
			c.logger.Debugf("using datastore %q", ds.Name)
			return object.NewDatastore(c.client.Client, ds.Reference()), nil
		}
	}
	return nil, errors.New("could not find an accessible datastore")
}

// uploadImage uploads an image from the given extracted OVA directory
// to a target URL.
func uploadImage(
	ctx context.Context,
	client *vim25.Client,
	item types.OvfFileItem,
	ovaDir string,
	targetURL *url.URL,
	updateStatus func(string),
	updateStatusInterval time.Duration,
	leaseUpdaterContext leaseUpdaterContext,
	clock clock.Clock,
	logger loggo.Logger,
) error {
	sourcePath := filepath.Join(ovaDir, item.Path)
	f, err := os.Open(sourcePath)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()

	// Transfer upload progress to the updateStatus function.
	statusUpdater := statusUpdater{
		ch:       make(chan progress.Report),
		clock:    clock,
		logger:   logger,
		update:   updateStatus,
		action:   fmt.Sprintf("uploading %s", item.Path),
		interval: updateStatusInterval,
	}

	// Update the lease periodically.
	leaseUpdater := leaseUpdater{
		ch:                  make(chan progress.Report),
		clock:               clock,
		logger:              logger,
		ctx:                 ctx,
		leaseUpdaterContext: leaseUpdaterContext,
	}

	// Upload.
	opts := soap.Upload{
		ContentLength: item.Size,
		Progress:      progress.Tee(&statusUpdater, &leaseUpdater),
	}
	if item.Create {
		opts.Method = "PUT"
		opts.Headers = map[string]string{"Overwrite": "t"}
	} else {
		opts.Method = "POST"
		opts.Type = "application/x-vnd.vmware-streamVmdk"
	}
	doUpload := func() error {
		// NOTE(axw) client.Upload is not cancellable,
		// as there is no way to inject a context. We
		// should send a patch to govmomi to make it
		// cancellable.
		return errors.Trace(client.Upload(f, targetURL, &opts))
	}

	var site catacomb.Catacomb
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &site,
		Work: doUpload,
		Init: []worker.Worker{
			jujuworker.NewSimpleWorker(statusUpdater.loop),
			jujuworker.NewSimpleWorker(leaseUpdater.loop),
		},
	}); err != nil {
		return errors.Trace(err)
	}
	return site.Wait()
}

type statusUpdater struct {
	clock    clock.Clock
	logger   loggo.Logger
	ch       chan progress.Report
	update   func(string)
	action   string
	interval time.Duration
}

// Sink is part of the progress.Sinker interface.
func (u *statusUpdater) Sink() chan<- progress.Report {
	return u.ch
}

func (u *statusUpdater) loop(abort <-chan struct{}) error {
	timer := u.clock.NewTimer(u.interval)
	defer timer.Stop()
	var timerChan <-chan time.Time

	var message string
	for {
		select {
		case <-abort:
			return tomb.ErrDying
		case <-timerChan:
			u.update(message)
			timer.Reset(u.interval)
			timerChan = nil
		case report, ok := <-u.ch:
			if !ok {
				return nil
			}
			if err := report.Error(); err != nil {
				message = fmt.Sprintf("%s: %s", u.action, err)
			} else {
				message = fmt.Sprintf(
					"%s: %.2f%% (%s)",
					u.action,
					report.Percentage(),
					report.Detail(),
				)
			}
			timerChan = timer.Chan()
		}
	}
}

type leaseUpdaterContext struct {
	lease *object.HttpNfcLease
	start int64
	size  int64
	total int64
}

type leaseUpdater struct {
	clock  clock.Clock
	logger loggo.Logger
	ch     chan progress.Report
	ctx    context.Context
	leaseUpdaterContext
}

// Sink is part of the progress.Sinker interface.
func (u *leaseUpdater) Sink() chan<- progress.Report {
	return u.ch
}

func (u *leaseUpdater) loop(abort <-chan struct{}) error {
	const interval = 2 * time.Second
	timer := u.clock.NewTimer(interval)
	defer timer.Stop()

	var progress int32
	for {
		select {
		case <-abort:
			return tomb.ErrDying
		case report, ok := <-u.ch:
			if !ok {
				return nil
			}
			progress = u.progress(report.Percentage())
		case <-timer.Chan():
			if err := u.lease.HttpNfcLeaseProgress(u.ctx, progress); err != nil {
				// NOTE(axw) we don't bail on error here, in
				// case it's just a transient failure. If it
				// is not, we would expect the upload to fail
				// to abort anyway.
				u.logger.Debugf("failed to update lease progress: %v", err)
			}
			timer.Reset(interval)
		}
	}
}

// progress computes the overall progress based on the size of the items
// uploaded prior, and the upload percentage of the current item.
func (u *leaseUpdater) progress(pc float32) int32 {
	pos := float64(u.start) + (float64(pc) * float64(u.size) / 100)
	return int32((100 * pos) / float64(u.total))
}
