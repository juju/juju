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

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mutex"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
)

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
	vmdkFilename := path.Join(vmdkDirectory, args.OVASHA256+".vmdk")
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

	// Upload the VMDK, and then convert it to a disk.
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

	// Move the temporary VMDK into its target location.
	task, err := fileManager.MoveDatastoreFile(
		ctx,
		datastore.Path(tempFilename),
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
