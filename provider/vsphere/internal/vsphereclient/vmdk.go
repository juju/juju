// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mutex"
	"github.com/juju/utils/clock"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	//	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// prepareVMDK ensures that VMDK exists and then copies it
// for a specific machine.
func (c *Client) prepareVMDK(
	ctx context.Context,
	args CreateVirtualMachineParams,
	datastore *object.Datastore,
	datacenter *object.Datacenter,
	taskWaiter *taskWaiter,
) (datastorePath string, resultErr error) {
	originalPath, release, err := c.ensureVMDK(ctx, args, datastore, datacenter, taskWaiter)
	if err != nil {
		return "", err
	}
	defer release()
        
	newPath := datastore.Path(path.Join(args.Name, args.Name+"-imported.vmdk"))
	c.logger.Debugf("preparing VMDK %s from %s", newPath, originalPath)
	fileManager := object.NewFileManager(c.client.Client)
	if err := fileManager.MakeDirectory(ctx, datastore.Path(args.Name), datacenter, true); err != nil {
		return "", errors.Annotate(err, "creating image directory")
	}
	task, err := fileManager.CopyDatastoreFile(
		ctx,
		originalPath,
		datacenter,
		newPath,
		datacenter,
		true,
	)
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, err := taskWaiter.waitTask(ctx, task, "copying VMDK"); err != nil {
		return "", errors.Trace(err)
	}
	c.logger.Debugf("prepared VMDK %s from %s", newPath, originalPath)
	return newPath, nil
}

// ensureVMDK ensures that the VMDK contained within the OVA returned
// by args.ReadOVA is either already in the datastore, or else stores it.
//
// ensureVMDK takes a machine lock for using the VMDKs, and returns
// a function to release that lock. The caller must call the release
// function once it has finished using the VMDK.
func (c *Client) ensureVMDK(
	ctx context.Context,
	args CreateVirtualMachineParams,
	datastore *object.Datastore,
	datacenter *object.Datacenter,
	taskWaiter *taskWaiter,
) (datastorePath string, release func(), resultErr error) {

	// Each controller maintains its own image cache. All compute
	// provisioners (i.e. each model's) run on the same controller
	// machine, so taking a machine lock ensures that only one
	// process is updating VMDKs at the same time. We lock around
	// access to the series directory.
	mutexReleaser, err := mutex.Acquire(mutex.Spec{
		Name:  "juju-vsphere-" + args.Series,
		Clock: args.Clock,
		Delay: time.Second,
	})
	if err != nil {
		return "", nil, errors.Annotate(err, "acquiring lock")
	}
	defer func() {
		if release == nil {
			mutexReleaser.Release()
		}
	}()

	// First, check if the VMDK has already been cached. If it hasn't,
	// but the VMDK directory exists already, we delete it and recreate
	// it; this is to remove older VMDKs.
	vmdkDirectory := path.Join(args.VMDKDirectory, args.Series)
	// We need a different filename than just '.vmdk' so that we won't connect
	// old unconverted images. The SHA256 is of the original, unconverted file
	vmdkFilename := path.Join(vmdkDirectory, args.OVASHA256+"-converted.vmdk")
	vmdkDatastorePath := datastore.Path(vmdkFilename)
	dirDatastorePath := datastore.Path(vmdkDirectory)
	fileManager := object.NewFileManager(c.client.Client)
	if _, err := datastore.Stat(ctx, vmdkFilename); err != nil {
		switch errors.Cause(err).(type) {
		case object.DatastoreNoSuchFileError:
			// Image directory exists. Delete it so we remove any
			// existing, older VMDK, and then create it below.
			task, err := fileManager.DeleteDatastoreFile(ctx, dirDatastorePath, datacenter)
			if err != nil {
				return "", nil, errors.Trace(err)
			}
			if _, err := taskWaiter.waitTask(ctx, task, "deleting image directory"); err != nil {
				return "", nil, errors.Annotate(err, "deleting image directory")
			}
		case object.DatastoreNoSuchDirectoryError:
			// Image directory doesn't exist; create it below.
			break
		default:
			return "", nil, errors.Trace(err)
		}
		if err := fileManager.MakeDirectory(ctx, dirDatastorePath, datacenter, true); err != nil {
			return "", nil, errors.Annotate(err, "creating image directory")
		}
	} else {
		// The disk has already been uploaded.
		return vmdkDatastorePath, mutexReleaser.Release, nil
	}

	// Fetch the OVA, and decode in-memory to find the VMDK stream.
	// An OVA is a tar archive.
	ovaLocation, ovaReadCloser, err := args.ReadOVA()
	if err != nil {
		return "", nil, errors.Annotate(err, "fetching OVA")
	}
	defer ovaReadCloser.Close()

	sha256sum := sha256.New()
	ovaTarReader := tar.NewReader(io.TeeReader(ovaReadCloser, sha256sum))
	var vmdkSize int64
	for {
		header, err := ovaTarReader.Next()
		if err != nil {
			return "", nil, errors.Annotate(err, "reading OVA")
		}
		if strings.HasSuffix(header.Name, ".vmdk") {
			vmdkSize = header.Size
			break
		}
	}

	// Upload the VMDK
	tempFilename := vmdkFilename + ".tmp"
	c.logger.Debugf("uploading %s contents to %s", ovaLocation, tempFilename)
	if err := c.uploadToDatastore(
		ctx, ovaTarReader, vmdkSize, datastore, tempFilename,
		args.Clock, args.UpdateProgress, args.UpdateProgressInterval,
	); err != nil {
		return "", nil, errors.Annotate(err, "uploading VMDK to datastore")
	}

	// Finish reading the rest of the OVA, so we can compute the hash.
	if _, err := io.Copy(sha256sum, ovaReadCloser); err != nil {
		return "", nil, errors.Annotate(err, "reading OVA")
	}
	if fmt.Sprintf("%x", sha256sum.Sum(nil)) != args.OVASHA256 {
		return "", nil, errors.New("SHA-256 hash mismatch for OVA")
	}

	// Convert VMDK into internal format
	convertedPath, err := c.convertVMDK(ctx, args, datastore, taskWaiter, datastore.Path(tempFilename))
	if err != nil {
		return "", nil, errors.Trace(err)
	}


	// Move the temporary VMDK into its target location. We might need to recreate the directory first.
	if _, err := datastore.Stat(ctx, vmdkDatastorePath); err != nil {
		switch errors.Cause(err).(type) {
		case object.DatastoreNoSuchFileError:
			break
		case object.DatastoreNoSuchDirectoryError:
			if err := fileManager.MakeDirectory(ctx, dirDatastorePath, datacenter, true); err != nil {
				return "", nil, errors.Annotate(err, "creating image directory")
			}
		default:
			return "", nil, errors.Trace(err)
		}
	}
	c.logger.Debugf("XXX %+q %+q %+q %+q", ctx, convertedPath, datacenter, vmdkDatastorePath)
	task, err := fileManager.MoveDatastoreFile(
		ctx,
		convertedPath,
		datacenter,
		vmdkDatastorePath,
		datacenter,
		true,
	)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	if _, err := taskWaiter.waitTask(ctx, task, "moving VMDK"); err != nil {
		return "", nil, errors.Trace(err)
	}
	return vmdkDatastorePath, mutexReleaser.Release, nil
}

// convertVMDK converts the disk the disk to ESXi format, destroying
// the original in the process.
// (wpk) That's an ugly procedure but I couldn't find a better one:
// We create a VM with this disk, then clone it, and detach disk
// from the cloned one - as it got converted to internal ESXi format.
// If you know any nicer way please, please do reimplement it.
func (c *Client) convertVMDK(
	ctx context.Context,
	args CreateVirtualMachineParams,
	datastore *object.Datastore,
	taskWaiter *taskWaiter,
	vmdkDatastorePath string,
) (string, error) {
	ovfManager := ovf.NewManager(c.client.Client)
	resourcePool := object.NewResourcePool(c.client.Client, *args.ComputeResource.ResourcePool)
	cisp := types.OvfCreateImportSpecParams{
		EntityName: args.Name + "temporaryImport",
	}

	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	folderPath := path.Join(folders.VmFolder.InventoryPath, args.Folder)
	vmFolder, err := finder.Folder(ctx, folderPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	
	spec, err := ovfManager.CreateImportSpec(ctx, SimpleOVF, resourcePool, datastore, cisp)
	if err != nil {
		return "", errors.Trace(err)
	} else if spec.Error != nil {
		return "", errors.New(spec.Error[0].LocalizedMessage)
	}
	importSpec := spec.ImportSpec.(*types.VirtualMachineImportSpec)
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec

	if err := c.addRootDisk(s, datastore, vmdkDatastorePath); err != nil {
		return "", errors.Trace(err)
	}
	
	lease, err := resourcePool.ImportVApp(ctx, importSpec, vmFolder, nil)
	if err != nil {
		return "", errors.Annotatef(err, "failed to import vapp")
	}
	info, err := lease.Wait(ctx, nil)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := lease.Complete(ctx); err != nil {
		return "", errors.Trace(err)
	}
	
	tempVm := object.NewVirtualMachine(c.client.Client, info.Entity)
	defer func() {
		// This should also take care of deleting the original image.
		if err := c.destroyVM(ctx, tempVm, taskWaiter); err != nil {
			c.logger.Warningf("failed to delete temporary VM: %s", err)
		}
	}()
	
	clonedVm, err := c.cloneVM(ctx, tempVm, args.Name, vmFolder, taskWaiter)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() {
		if err := c.destroyVM(ctx, clonedVm, taskWaiter); err != nil {
			c.logger.Warningf("failed to delete temporary VM: %s", err)
		}
	}()

	convertedPath, err := c.detachDisk(ctx, clonedVm, taskWaiter)
	if err != nil {
		return "", errors.Trace(err)
	}
	return convertedPath, nil
}

func (c *Client) uploadToDatastore(
	ctx context.Context,
	r io.Reader,
	size int64,
	datastore *object.Datastore,
	filename string,
	clock clock.Clock,
	updateProgress func(string),
	updateProgressInterval time.Duration,
) error {
	var err error
	withStatusUpdater(
		ctx, fmt.Sprintf("uploading %s", filename),
		clock, updateProgress, updateProgressInterval,
		func(ctx context.Context, s progress.Sinker) {
			p := soap.DefaultUpload
			p.Progress = s
			p.ContentLength = size
			err = datastore.Upload(ctx, r, filename, &p)
		},
	)
	return errors.Annotate(err, "uploading VMDK to datastore")
}
