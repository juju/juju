/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package importx

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/juju/govmomi/govc/cli"
	"github.com/juju/govmomi/govc/flags"
	"github.com/juju/govmomi/object"
	"github.com/juju/govmomi/vim25"
	"github.com/juju/govmomi/vim25/progress"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"
	"golang.org/x/net/context"
)

type ovf struct {
	*flags.DatastoreFlag
	*flags.ResourcePoolFlag
	*flags.HostSystemFlag
	*flags.OutputFlag

	Client       *vim25.Client
	Datacenter   *object.Datacenter
	Datastore    *object.Datastore
	ResourcePool *object.ResourcePool

	Archive
}

func init() {
	cli.Register("import.ovf", &ovf{})
}

func (cmd *ovf) Register(f *flag.FlagSet) {}

func (cmd *ovf) Process() error { return nil }

func (cmd *ovf) Usage() string {
	return "PATH_TO_OVF"
}

func (cmd *ovf) Run(f *flag.FlagSet) error {
	file, err := cmd.Prepare(f)

	if err != nil {
		return err
	}

	cmd.Archive = &FileArchive{file}

	return cmd.Import(file)
}

func (cmd *ovf) Prepare(f *flag.FlagSet) (string, error) {
	var err error

	args := f.Args()
	if len(args) != 1 {
		return "", errors.New("no file to import")
	}

	cmd.Client, err = cmd.DatastoreFlag.Client()
	if err != nil {
		return "", err
	}

	cmd.Datacenter, err = cmd.DatastoreFlag.Datacenter()
	if err != nil {
		return "", err
	}

	cmd.Datastore, err = cmd.DatastoreFlag.Datastore()
	if err != nil {
		return "", err
	}

	cmd.ResourcePool, err = cmd.ResourcePoolFlag.ResourcePool()
	if err != nil {
		return "", err
	}

	return f.Arg(0), nil
}

func (cmd *ovf) ReadAll(name string) ([]byte, error) {
	f, _, err := cmd.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ioutil.ReadAll(f)
}

func (cmd *ovf) Import(fpath string) error {
	c := cmd.Client

	desc, err := cmd.ReadAll(fpath)
	if err != nil {
		return err
	}

	// extract name from .ovf for use as VM name
	ovf := struct {
		VirtualSystem struct {
			Name string
		}
	}{}

	if err := xml.Unmarshal(desc, &ovf); err != nil {
		return fmt.Errorf("failed to parse ovf: %s", err.Error())
	}

	cisp := types.OvfCreateImportSpecParams{
		EntityName: ovf.VirtualSystem.Name,
		OvfManagerCommonParams: types.OvfManagerCommonParams{
			Locale: "US",
		},
	}

	m := object.NewOvfManager(c)
	spec, err := m.CreateImportSpec(context.TODO(), string(desc), cmd.ResourcePool, cmd.Datastore, cisp)
	if err != nil {
		return err
	}

	if spec.Error != nil {
		return errors.New(spec.Error[0].LocalizedMessage)
	}

	if spec.Warning != nil {
		for _, w := range spec.Warning {
			_, _ = cmd.Log(fmt.Sprintf("Warning: %s\n", w.LocalizedMessage))
		}
	}

	// TODO: ImportSpec may have unitNumber==0, but this field is optional in the wsdl
	// and hence omitempty in the struct tag; but unitNumber is required for certain devices.
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec
	for _, d := range s.DeviceChange {
		n := &d.GetVirtualDeviceConfigSpec().Device.GetVirtualDevice().UnitNumber
		if *n == 0 {
			*n = -1
		}
	}

	var host *object.HostSystem
	if cmd.SearchFlag.IsSet() {
		if host, err = cmd.HostSystem(); err != nil {
			return err
		}
	}

	// TODO: need a folder option
	folders, err := cmd.Datacenter.Folders(context.TODO())
	if err != nil {
		return err
	}

	lease, err := cmd.ResourcePool.ImportVApp(context.TODO(), spec.ImportSpec, folders.VmFolder, host)
	if err != nil {
		return err
	}

	info, err := lease.Wait(context.TODO())
	if err != nil {
		return err
	}

	// Build slice of items and URLs first, so that the lease updater can know
	// about every item that needs to be uploaded, and thereby infer progress.
	var items []ovfFileItem

	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}

			u, err := c.Client.ParseURL(device.Url)
			if err != nil {
				return err
			}

			i := ovfFileItem{
				url:  u,
				item: item,
				ch:   make(chan progress.Report),
			}

			items = append(items, i)
		}
	}

	u := newLeaseUpdater(cmd.Client, lease, items)
	defer u.Done()

	for _, i := range items {
		err = cmd.Upload(lease, i)
		if err != nil {
			return err
		}
	}

	return lease.HttpNfcLeaseComplete(context.TODO())
}

func (cmd *ovf) Upload(lease *object.HttpNfcLease, ofi ovfFileItem) error {
	item := ofi.item
	file := item.Path

	f, size, err := cmd.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	logger := cmd.ProgressLogger(fmt.Sprintf("Uploading %s... ", path.Base(file)))
	defer logger.Wait()

	opts := soap.Upload{
		ContentLength: size,
		Progress:      progress.Tee(ofi, logger),
	}

	// Non-disk files (such as .iso) use the PUT method.
	// Overwrite: t header is also required in this case (ovftool does the same)
	if item.Create {
		opts.Method = "PUT"
		opts.Headers = map[string]string{
			"Overwrite": "t",
		}
	} else {
		opts.Method = "POST"
		opts.Type = "application/x-vnd.vmware-streamVmdk"
	}

	return cmd.Client.Client.Upload(f, ofi.url, &opts)
}
