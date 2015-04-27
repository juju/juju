// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/govmomi/object"
	"github.com/juju/govmomi/vim25/progress"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"

	"github.com/juju/juju/instance"
)

/*
This file contains implementation of the process of importing OVF template using vsphere API.  This process can be splitted in the following steps
1. Download OVF template
2. Call CreateImportSpec method from vsphere API https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/. This method validates the OVF descriptor against the hardware supported by the host system. If the validation succeeds, return a result containing:
  * An ImportSpec to use when importing the entity.
  * A list of items to upload (for example disk backing files, ISO images etc.)
3. Prepare all necessary parameters (CPU, mem, etc.) and call ImportVApp method https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/. This method is responsible for actually creating VM. This method return HttpNfcLease (https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.HttpNfcLease.html) object, that is used to monitor status of the process.
4. Upload virtual disk contents (that usually consist of a single vmdk file)
5. Call HttpNfcLeaseComplete https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/ and indicate that the process of uploading is finished - this step finishes the process.
*/

//this type implements progress.Sinker interface, that is requred to obtain the status of uploading an item to vspehere
type ovfFileItem struct {
	url  *url.URL
	item types.OvfFileItem
	ch   chan progress.Report
}

func (o ovfFileItem) Sink() chan<- progress.Report {
	return o.ch
}

type ovfImportManager struct {
	client *client
}

func (m *ovfImportManager) importOvf(machineID string, zone *vmwareAvailZone, hwc *instance.HardwareCharacteristics, img *OvfFileMetadata, userData []byte, sshKey string, isState bool) (*object.VirtualMachine, error) {
	folders, err := m.client.datacenter.Folders(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}

	ovf, err := m.downloadOvf(img.Url)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cisp := types.OvfCreateImportSpecParams{
		EntityName: machineID,
		PropertyMapping: []types.KeyValue{
			types.KeyValue{Key: "public-keys", Value: sshKey},
			types.KeyValue{Key: "user-data", Value: base64.StdEncoding.EncodeToString(userData)},
		},
	}

	ovfManager := object.NewOvfManager(m.client.connection.Client)
	resourcePool := object.NewReference(m.client.connection.Client, *zone.r.ResourcePool)
	datastore := object.NewReference(m.client.connection.Client, zone.r.Datastore[0])
	spec, err := ovfManager.CreateImportSpec(context.TODO(), string(ovf), resourcePool, datastore, cisp)
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
	if isState {
		s.ExtraConfig = append(s.ExtraConfig, &types.OptionValue{Key: metadataKeyIsState, Value: metadataValueIsState})
	}
	for _, d := range s.DeviceChange {
		if disk, ok := d.GetVirtualDeviceConfigSpec().Device.(*types.VirtualDisk); ok {
			disk.CapacityInKB = int64(*hwc.RootDisk * 1024)
		}
		//Set UnitNumber to -1 if it is unset in ovf file template (in this case it is parces as 0)
		//but 0 causes an error for some devices
		n := &d.GetVirtualDeviceConfigSpec().Device.GetVirtualDevice().UnitNumber
		if *n == 0 {
			*n = -1
		}
	}
	rp := object.NewResourcePool(m.client.connection.Client, *zone.r.ResourcePool)
	lease, err := rp.ImportVApp(context.TODO(), spec.ImportSpec, folders.VmFolder, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to import vapp")
	}

	info, err := lease.Wait(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	items := []ovfFileItem{}
	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}

			u, err := m.client.connection.Client.ParseURL(device.Url)
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
		err = m.uploadImage(i, img.Url[:ind])
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lease.HttpNfcLeaseComplete(context.TODO())
	return object.NewVirtualMachine(m.client.connection.Client, info.Entity), nil
}

func (m *ovfImportManager) downloadOvf(url string) (string, error) {
	logger.Debugf("Downloading ovf file from url: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("can't download ovf file from url: %s, status: %s", url, resp.StatusCode)
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(bytes), nil
}

func (m *ovfImportManager) uploadImage(ofi ovfFileItem, baseUrl string) error {
	f, err := m.downloadFileItem(strings.Join([]string{baseUrl, ofi.item.Path}, "/"))
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
	logger.Debugf("Uploading image to %s", ofi.url)
	go func() {
		lastPercent := 0
		for pr := <-ofi.ch; pr != nil; pr = <-ofi.ch {
			curPercent := int(pr.Percentage())
			if curPercent-lastPercent >= 10 {
				lastPercent = curPercent
				logger.Debugf("Progress: %d%", lastPercent)
			}
		}
	}()
	err = m.client.connection.Client.Upload(f, ofi.url, &opts)
	if err == nil {
		logger.Debugf("Image uploaded")
	} else {
		err = errors.Trace(err)
	}

	return err
}

func (m *ovfImportManager) downloadFileItem(url string) (io.ReadCloser, error) {
	logger.Debugf("Downloading file from url %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, errors.Errorf("can't download file from url: %s, status: %s", url, resp.StatusCode)
	}

	return resp.Body, nil
}
