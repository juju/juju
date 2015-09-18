// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"archive/tar"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/govmomi/object"
	"github.com/juju/govmomi/vim25/progress"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"
	"github.com/juju/juju/juju/osenv"
	"golang.org/x/net/context"
)

/*
This file contains implementation of the process of importing OVF template using vsphere API.  This process can be splitted in the following steps
1. Download OVA template
2. Extract it to a temp folder and load ovf file from it.
3. Call CreateImportSpec method from vsphere API https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/. This method validates the OVF descriptor against the hardware supported by the host system. If the validation succeeds, return a result containing:
  * An ImportSpec to use when importing the entity.
  * A list of items to upload (for example disk backing files, ISO images etc.)
4. Prepare all necessary parameters (CPU, mem, etc.) and call ImportVApp method https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/. This method is responsible for actually creating VM. This method return HttpNfcLease (https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.HttpNfcLease.html) object, that is used to monitor status of the process.
5. Upload virtual disk contents (that usually consist of a single vmdk file)
6. Call HttpNfcLeaseComplete https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/ and indicate that the process of uploading is finished - this step finishes the process.
*/

//this type implements progress.Sinker interface, that is requred to obtain the status of uploading an item to vspehere
type ovaFileItem struct {
	url  *url.URL
	item types.OvfFileItem
	ch   chan progress.Report
}

func (o ovaFileItem) Sink() chan<- progress.Report {
	return o.ch
}

type ovaImportManager struct {
	client *client
}

func (m *ovaImportManager) importOva(ecfg *environConfig, instSpec *instanceSpec) (*object.VirtualMachine, error) {
	folders, err := m.client.datacenter.Folders(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}

	basePath, err := ioutil.TempDir(osenv.JujuHome(), "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err := os.RemoveAll(basePath); err != nil {
			logger.Errorf("can't remove temp directory, error: %s", err.Error())
		}
	}()
	ovf, err := m.downloadOva(basePath, instSpec.img.Url)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cisp := types.OvfCreateImportSpecParams{
		EntityName: instSpec.machineID,
		PropertyMapping: []types.KeyValue{
			types.KeyValue{Key: "public-keys", Value: instSpec.sshKey},
			types.KeyValue{Key: "user-data", Value: base64.StdEncoding.EncodeToString(instSpec.userData)},
		},
	}

	ovfManager := object.NewOvfManager(m.client.connection.Client)
	resourcePool := object.NewReference(m.client.connection.Client, *instSpec.zone.r.ResourcePool)
	datastore := object.NewReference(m.client.connection.Client, instSpec.zone.r.Datastore[0])
	spec, err := ovfManager.CreateImportSpec(context.TODO(), string(ovf), resourcePool, datastore, cisp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec
	s.NumCPUs = int(*instSpec.hwc.CpuCores)
	s.MemoryMB = int64(*instSpec.hwc.Mem)
	s.CpuAllocation = &types.ResourceAllocationInfo{
		Limit:       int64(*instSpec.hwc.CpuPower),
		Reservation: int64(*instSpec.hwc.CpuPower),
	}
	if instSpec.isState {
		s.ExtraConfig = append(s.ExtraConfig, &types.OptionValue{Key: metadataKeyIsState, Value: metadataValueIsState})
	}
	for _, d := range s.DeviceChange {
		if disk, ok := d.GetVirtualDeviceConfigSpec().Device.(*types.VirtualDisk); ok {
			if disk.CapacityInKB < int64(*instSpec.hwc.RootDisk*1024) {
				disk.CapacityInKB = int64(*instSpec.hwc.RootDisk * 1024)
			}
			//Set UnitNumber to -1 if it is unset in ovf file template (in this case it is parces as 0)
			//but 0 causes an error for disk devices
			if disk.UnitNumber == 0 {
				disk.UnitNumber = -1
			}
		}
	}
	if ecfg.externalNetwork() != "" {
		s.DeviceChange = append(s.DeviceChange, &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualE1000{
				VirtualEthernetCard: types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Backing: &types.VirtualEthernetCardNetworkBackingInfo{
							VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
								DeviceName: ecfg.externalNetwork(),
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
	rp := object.NewResourcePool(m.client.connection.Client, *instSpec.zone.r.ResourcePool)
	lease, err := rp.ImportVApp(context.TODO(), spec.ImportSpec, folders.VmFolder, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to import vapp")
	}

	info, err := lease.Wait(context.TODO())
	if err != nil {
		return nil, errors.Trace(err)
	}
	items := []ovaFileItem{}
	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}

			u, err := m.client.connection.Client.ParseURL(device.Url)
			if err != nil {
				return nil, errors.Trace(err)
			}

			i := ovaFileItem{
				url:  u,
				item: item,
				ch:   make(chan progress.Report),
			}
			items = append(items, i)
		}
	}

	for _, i := range items {
		err = m.uploadImage(i, basePath)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lease.HttpNfcLeaseComplete(context.TODO())
	return object.NewVirtualMachine(m.client.connection.Client, info.Entity), nil
}

func (m *ovaImportManager) downloadOva(basePath, url string) (string, error) {
	logger.Debugf("Downloading ova file from url: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("can't download ova file from url: %s, status: %d", url, resp.StatusCode)
	}

	ovfFilePath, err := m.extractOva(basePath, resp.Body)
	if err != nil {
		return "", errors.Trace(err)
	}

	file, err := os.Open(ovfFilePath)
	defer file.Close()
	if err != nil {
		return "", errors.Trace(err)
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(bytes), nil
}

func (m *ovaImportManager) extractOva(basePath string, body io.Reader) (string, error) {
	logger.Debugf("Extracting ova to path: %s", basePath)
	tarBallReader := tar.NewReader(body)
	var ovfFileName string

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", errors.Trace(err)
		}
		filename := header.Name
		if filepath.Ext(filename) == ".ovf" {
			ovfFileName = filename
		}
		logger.Debugf("Writing file %s", filename)
		err = func() error {
			writer, err := os.Create(filepath.Join(basePath, filename))
			defer writer.Close()
			if err != nil {
				return errors.Trace(err)
			}
			_, err = io.Copy(writer, tarBallReader)
			if err != nil {
				return errors.Trace(err)
			}
			return nil
		}()
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	if ovfFileName == "" {
		return "", errors.Errorf("no ovf file found in the archive")
	}
	logger.Debugf("Ova extracted successfully")
	return filepath.Join(basePath, ovfFileName), nil
}

func (m *ovaImportManager) uploadImage(ofi ovaFileItem, basePath string) error {
	filepath := filepath.Join(basePath, ofi.item.Path)
	logger.Debugf("Uploading item from path: %s", filepath)
	f, err := os.Open(filepath)
	if err != nil {
		return errors.Trace(err)
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
				logger.Debugf("Progress: %d%%", lastPercent)
			}
		}
	}()
	err = m.client.connection.Client.Upload(f, ofi.url, &opts)
	if err == nil {
		logger.Debugf("Image uploaded")
	}
	return errors.Trace(err)
}
