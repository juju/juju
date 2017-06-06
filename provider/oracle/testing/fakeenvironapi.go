// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"

	"github.com/juju/juju/provider/oracle"
)

type FakeInstancer struct {
	Instance    response.Instance
	InstanceErr error
}

func (f FakeInstancer) InstanceDetails(string) (response.Instance, error) {
	return f.Instance, f.InstanceErr
}

type FakeInstance struct {
	Create    response.LaunchPlan
	CreateErr error
	All       response.AllInstances
	AllErr    error
	DeleteErr error
}

func (f FakeInstance) CreateInstance(api.InstanceParams) (response.LaunchPlan, error) {
	return f.Create, f.CreateErr
}

func (f FakeInstance) AllInstances([]api.Filter) (response.AllInstances, error) {
	return f.All, f.AllErr
}

func (f FakeInstance) DeleteInstance(string) error {
	return f.DeleteErr
}

type FakeAuthenticater struct {
	AuthenticateErr error
}

func (f FakeAuthenticater) Authenticate() error {
	return f.AuthenticateErr
}

type FakeShaper struct {
	All    response.AllShapes
	AllErr error
}

func (f FakeShaper) AllShapes([]api.Filter) (response.AllShapes, error) {
	return f.All, f.AllErr
}

type FakeImager struct {
	All    response.AllImageLists
	AllErr error
}

func (f FakeImager) AllImageLists([]api.Filter) (response.AllImageLists, error) {
	return f.All, f.AllErr
}

func (f FakeImager) CreateImageList(def int, description string, name string) (resp response.ImageList, err error) {
	return response.ImageList{}, nil
}

func (f FakeImager) CreateImageListEntry(
	name string,
	attributes map[string]interface{},
	version int,
	machineImages []string,
) (resp response.ImageListEntryAdd, err error) {

	return response.ImageListEntryAdd{}, nil
}

func (f FakeImager) DeleteImageList(name string) (err error) {
	return nil
}

type FakeIpReservation struct {
	All       response.AllIpReservations
	AllErr    error
	Update    response.IpReservation
	UpdateErr error
	Create    response.IpReservation
	CreateErr error
	DeleteErr error
}

func (f FakeIpReservation) AllIpReservations([]api.Filter) (response.AllIpReservations, error) {
	return f.All, f.AllErr
}

func (f FakeIpReservation) UpdateIpReservation(string, string, common.IPPool, bool, []string) (response.IpReservation, error) {
	return f.Update, f.UpdateErr
}

func (f FakeIpReservation) CreateIpReservation(string, common.IPPool, bool, []string) (response.IpReservation, error) {
	return f.Create, f.CreateErr
}

func (f FakeIpReservation) DeleteIpReservation(string) error {
	return f.DeleteErr
}

type FakeIpAssociation struct {
	Create    response.IpAssociation
	CreateErr error
	DeleteErr error
	All       response.AllIpAssociations
	AllErr    error
}

func (f FakeIpAssociation) CreateIpAssociation(common.IPPool,
	common.VcableID) (response.IpAssociation, error) {
	return f.Create, f.CreateErr
}

func (f FakeIpAssociation) DeleteIpAssociation(string) error {
	return f.DeleteErr
}

func (f FakeIpAssociation) AllIpAssociations([]api.Filter) (response.AllIpAssociations, error) {
	return f.All, f.AllErr
}

type FakeIpNetworkExchanger struct {
	All    response.AllIpNetworkExchanges
	AllErr error
}

func (f FakeIpNetworkExchanger) AllIpNetworkExchanges([]api.Filter) (response.AllIpNetworkExchanges, error) {
	return f.All, f.AllErr
}

type FakeIpNetworker struct {
	All    response.AllIpNetworks
	AllErr error
}

func (f FakeIpNetworker) AllIpNetworks([]api.Filter) (response.AllIpNetworks, error) {
	return f.All, f.AllErr
}

type FakeVnicSet struct {
	Create     response.VnicSet
	CreateErr  error
	VnicSet    response.VnicSet
	VnicSetErr error
	DeleteErr  error
}

func (f FakeVnicSet) DeleteVnicSet(string) error {
	return f.DeleteErr
}

func (f FakeVnicSet) CreateVnicSet(api.VnicSetParams) (response.VnicSet, error) {
	return f.Create, f.CreateErr
}

func (f FakeVnicSet) VnicSetDetails(string) (response.VnicSet, error) {
	return f.VnicSet, f.VnicSetErr
}

type FakeEnvironAPI struct {
	FakeInstancer
	FakeInstance
	FakeAuthenticater
	FakeImager
	FakeIpReservation
	FakeIpAssociation
	FakeIpNetworkExchanger
	FakeIpNetworker
	FakeVnicSet
	FakeShaper

	FakeStorageAPI

	FakeRules
	FakeAcl
	FakeSecIp
	FakeIpAddressprefixSet
	FakeSecList
	FakeSecRules
	FakeApplication
	FakeAssociation
}

var _ oracle.EnvironAPI = (*FakeEnvironAPI)(nil)

var (
	DefaultFakeInstancer = FakeInstancer{
		Instance: response.Instance{
			Domain: "compute-a432100.oraclecloud.internal.",
			Placement_requirements: []string{
				"/system/compute/placement/default",
				"/system/compute/allow_instances",
			},
			Ip:          "10.31.5.106",
			Fingerprint: "",
			Site:        "",
			Last_state_change_time: interface{}(nil),
			Error_exception:        interface{}(nil),
			Cluster:                interface{}(nil),
			Shape:                  "oc5",
			Start_requested:        false,
			Vethernets:             interface{}(nil),
			Imagelist:              "",
			Image_format:           "raw",
			Cluster_uri:            interface{}(nil),
			Relationships:          []string{},
			Target_node:            interface{}(nil),
			Availability_domain:    "/uscom-central-1a",
			Networking: common.Networking{
				"eth0": common.Nic{
					Dns: []string{
						"ea63b8.compute-a432100.oraclecloud.internal.",
					},
					Model: "",
					Nat:   "ippool:/oracle/public/ippool",
					Seclists: []string{
						"/Compute-a432100/default/default",
					},
					Vethernet: "/oracle/public/default",
					Vnic:      "",
					Ipnetwork: "",
				},
			},
			Seclist_associations: interface{}(nil),
			Hostname:             "ea63b8.compute-a432100.oraclecloud.internal.",
			State:                "running",
			Disk_attach:          "",
			Label:                "tools",
			Priority:             "/oracle/public/default",
			Platform:             "linux",
			Quota_reservation:    interface{}(nil),
			Suspend_file:         interface{}(nil),
			Node:                 interface{}(nil),
			Resource_requirements: response.ResourceRequirments{
				Compressed_size:   0x0,
				Is_root_ssd:       false,
				Ram:               0x0,
				Cpus:              0,
				Root_disk_size:    0x0,
				Io:                0x0,
				Decompressed_size: 0x0,
				Gpus:              0x0,
				Ssd_data_size:     0x0,
			},
			Virtio:        interface{}(nil),
			Vnc:           "10.31.5.105:5900",
			Desired_state: "running",
			Storage_attachments: []response.Storage{
				response.Storage{
					Index:               0x1,
					Storage_volume_name: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
					Name:                "/Compute-a432100/sgiulitti@cloudbase.com/0/1f90e657-f852-45ad-afbf-9a94f640a7ae",
				},
			},
			Start_time: "2017-03-11T11:07:14Z", Storage_attachment_associations: []interface{}(nil),
			Quota: "/Compute-a432100", Vnc_key: interface{}(nil),
			Numerical_priority: 0x0,
			Suspend_requested:  false,
			Entry:              0,
			Error_reason:       "",
			Nat_associations:   interface{}(nil),
			SSHKeys: []string{
				"/Compute-a432100/sgiulitti@cloudbase.com/sgiulitti",
			},
			Tags: []string{
				"vm-dev",
				"juju-tools",
				"toolsdir",
				"/Compute-a432100/sgiulitti@cloudbase.com/0",
				"16a4e037dd2068fe691aed9ed0c40460",
			},
			Resolvers: interface{}(nil),
			Metrics:   interface{}(nil),
			Account:   "/Compute-a432100/default",
			Node_uuid: interface{}(nil),
			Name:      "/Compute-a432100/sgiulitti@cloudbase.com/0/ebc4ce91-56bb-4120-ba78-13762597f837",
			Vcable_id: "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6",
			Higgs:     interface{}(nil),
			Hypervisor: response.Hypervisor{
				Mode: "hvm",
			},
			Uri:              "https://compute.uscom-central-1.oraclecloud.com/instance/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837",
			Console:          interface{}(nil),
			Reverse_dns:      true,
			Launch_context:   "",
			Delete_requested: interface{}(nil),
			Tracking_id:      interface{}(nil),
			Hypervisor_type:  interface{}(nil),
			Attributes: response.Attributes{
				Dns: map[string]string{
					"domain":              "compute-a432100.oraclecloud.internal.",
					"hostname":            "ea63b8.compute-a432100.oraclecloud.internal.",
					"nimbula_vcable-eth0": "ea63b8.compute-a432100.oraclecloud.internal.",
				},
				Network: map[string]response.Network{
					"nimbula_vcable-eth0": response.Network{
						Address: []string{
							"c6:b0:fd:f1:ef:ac",
							"10.31.5.106",
						},
						Dhcp_options:   []string{},
						Id:             "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6",
						Model:          "",
						Vethernet:      "/oracle/public/default",
						Vethernet_id:   "0",
						Vethernet_type: "vlan",
						Instance:       "",
						Ipassociations: []string(nil),
						Ipattachment:   "",
						Ipnetwork:      "",
						Vnic:           "",
						Vnicsets:       []string(nil),
					},
				},
				Nimbula_orchestration: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_instance",
				Sshkeys: []string{
					"ssh-rsa AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
				},
				Userdata: map[string]interface{}{}},
			Boot_order: []int{1},
			Last_seen:  interface{}(nil),
		},
	}

	DefaultEnvironAPI = &FakeEnvironAPI{
		FakeInstancer: DefaultFakeInstancer,
		FakeInstance: FakeInstance{
			Create: response.LaunchPlan{
				Relationships: nil,
				Instances: []response.Instance{
					DefaultFakeInstancer.Instance,
				},
			},
			CreateErr: nil,
			All: response.AllInstances{
				Result: []response.Instance{
					DefaultFakeInstancer.Instance,
				},
			},
			AllErr:    nil,
			DeleteErr: nil,
		},
		FakeAuthenticater: FakeAuthenticater{
			AuthenticateErr: nil,
		},
		FakeShaper: FakeShaper{
			All: response.AllShapes{
				Result: []response.Shape{
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0xf000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x174876e8000,
						Gpus:           0x0,
						Cpus:           8,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/ocio3m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "ocio3m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x7800,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           4,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc2m",
						Root_disk_size: 0x0,
						Io:             0x190,
						Name:           "oc2m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0xf000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           8,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc3m",
						Root_disk_size: 0x0,
						Io:             0x258,
						Name:           "oc3m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c00,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           4,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc4",
						Root_disk_size: 0x0,
						Io:             0x190,
						Name:           "oc4",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0xf000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           16,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc6",
						Root_disk_size: 0x0,
						Io:             0x320,
						Name:           "oc6",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x5d21dba0000,
						Gpus:           0x0,
						Cpus:           32,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/ocio5m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "ocio5m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x7800,
						Is_root_ssd:    false,
						Ssd_data_size:  0xba43b74000,
						Gpus:           0x0,
						Cpus:           4,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/ocio2m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "ocio2m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x1e00,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           2,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc3",
						Root_disk_size: 0x0,
						Io:             0xc8,
						Name:           "oc3",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x7800,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           8,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc5",
						Root_disk_size: 0x0,
						Io:             0x258,
						Name:           "oc5",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x1e000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           32,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc7",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc7",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x2d000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           48,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc8",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc8",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x1e000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           16,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc4m",
						Root_disk_size: 0x0,
						Io:             0x320,
						Name:           "oc4m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           32,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc5m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc5m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c00,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           2,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc1m",
						Root_disk_size: 0x0,
						Io:             0xc8,
						Name:           "oc1m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x78000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           64,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc9m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc9m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x5a000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           48,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc8m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc8m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x0,
						Gpus:           0x0,
						Cpus:           64,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/oc9",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "oc9",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x3c00,
						Is_root_ssd:    false,
						Ssd_data_size:  0x5d21dba000,
						Gpus:           0x0,
						Cpus:           2,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/ocio1m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "ocio1m",
					},
					response.Shape{
						Nds_iops_limit: 0x0,
						Ram:            0x1e000,
						Is_root_ssd:    false,
						Ssd_data_size:  0x2e90edd0000,
						Gpus:           0x0,
						Cpus:           16,
						Uri:            "https://compute.uscom-central-1.oraclecloud.com/shape/ocio4m",
						Root_disk_size: 0x0,
						Io:             0x3e8,
						Name:           "ocio4m",
					},
				},
			},
			AllErr: nil,
		},
		FakeImager: FakeImager{
			All: response.AllImageLists{
				Result: []response.ImageList{
					response.ImageList{
						Default:     1,
						Description: nil,
						Entries: []response.ImageListEntry{
							response.ImageListEntry{
								Attributes: response.AttributesEntry{
									Userdata:        map[string]interface{}{"enable_rdp": "true"},
									MinimumDiskSize: "27",
									DefaultShape:    "oc4",
									SupportedShapes: "oc1m,oc2m,oc3,oc3m,oc4,oc4m,oc5,oc5m,oc6,oc7,ocio1m,ocio2m,ocio3m,ocio4m,ocio5m,ociog1k80,ociog2k80,ociog3k80"},
								Imagelist:     "",
								Version:       1,
								Machineimages: []string{"/Compute-a432100/sgiulitti@cloudbase.com/Microsoft_Windows_Server_2012_R2"},
								Uri:           "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Microsoft_Windows_Server_2012_R2/entry/1",
							},
						},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Microsoft_Windows_Server_2012_R2",
						Name: "/Compute-a432100/sgiulitti@cloudbase.com/Microsoft_Windows_Server_2012_R2",
					},
					response.ImageList{
						Default:     1,
						Description: nil,
						Entries: []response.ImageListEntry{
							response.ImageListEntry{
								Attributes: response.AttributesEntry{
									Userdata:        map[string]interface{}{},
									MinimumDiskSize: "10",
									DefaultShape:    "oc2m",
									SupportedShapes: "oc3,oc4,oc5,oc6,oc7,oc1m,oc2m,oc3m,oc4m,oc5m",
								},
								Imagelist:     "",
								Version:       1,
								Machineimages: []string{"/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307"},
								Uri:           "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307/entry/1",
							},
						},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
						Name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
					},
					response.ImageList{
						Default:     1,
						Description: nil,
						Entries: []response.ImageListEntry{
							response.ImageListEntry{
								Attributes: response.AttributesEntry{
									Userdata:        map[string]interface{}{},
									MinimumDiskSize: "10",
									DefaultShape:    "oc2m",
									SupportedShapes: "oc3,oc4,oc5,oc6,oc7,oc1m,oc2m,oc3m,oc4m,oc5m",
								},
								Imagelist: "",
								Version:   1,
								Machineimages: []string{
									"/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.14.04-LTS.amd64.20170307",
								},
								Uri: "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.14.04-LTS.amd64.20170307/entry/1",
							},
						},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.14.04-LTS.amd64.20170307",
						Name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.14.04-LTS.amd64.20170307",
					},
					response.ImageList{
						Default:     1,
						Description: nil,
						Entries: []response.ImageListEntry{
							response.ImageListEntry{
								Attributes: response.AttributesEntry{
									Userdata:        map[string]interface{}{},
									MinimumDiskSize: "10",
									DefaultShape:    "oc2m",
									SupportedShapes: "oc3,oc4,oc5,oc6,oc7,oc1m,oc2m,oc3m,oc4m,oc5m",
								},
								Imagelist: "",
								Version:   1,
								Machineimages: []string{
									"/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.12.04-LTS.amd64.20170307",
								},
								Uri: "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.12.04-LTS.amd64.20170307/entry/1",
							},
						},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Ubuntu.12.04-LTS.amd64.20170307",
						Name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.12.04-LTS.amd64.20170307",
					},
					response.ImageList{
						Default:     1,
						Description: nil,
						Entries: []response.ImageListEntry{
							response.ImageListEntry{
								Attributes: response.AttributesEntry{
									Userdata:        map[string]interface{}{"enable_rdp": "true"},
									MinimumDiskSize: "27",
									DefaultShape:    "oc4",
									SupportedShapes: "oc1m,oc2m,oc3,oc3m,oc4,oc4m,oc5,oc5m,oc6,oc7,ocio1m,ocio2m,ocio3m,ocio4m,ocio5m,ociog1k80,ociog2k80,ociog3k80",
								},
								Imagelist: "",
								Version:   1,
								Machineimages: []string{
									"/Compute-a432100/sgiulitti@cloudbase.com/Microsoft_Windows_Server_2008_R2",
								},
								Uri: "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Microsoft_Windows_Server_2008_R2/entry/1",
							},
						},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com/imagelist/Compute-a432100/sgiulitti%40cloudbase.com/Microsoft_Windows_Server_2008_R2",
						Name: "/Compute-a432100/sgiulitti@cloudbase.com/Microsoft_Windows_Server_2008_R2",
					},
				},
			},
			AllErr: nil,
		},
		FakeIpReservation: FakeIpReservation{
			All: response.AllIpReservations{
				Result: []response.IpReservation{
					response.IpReservation{
						Account:    "/Compute-a432100/default",
						Ip:         "129.150.66.1",
						Name:       "/Compute-a432100/sgiulitti@cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
						Parentpool: "/oracle/public/ippool",
						Permanent:  false,
						Quota:      nil,
						Tags:       []string{},
						Uri:        "https://compute.uscom-central-1.oraclecloud.com/ip/reservation/Compute-a432100/sgiulitti%40cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
						Used:       true,
					},
				},
			},
			Update: response.IpReservation{
				Account:    "/Compute-a432100/default",
				Ip:         "129.150.66.1",
				Name:       "/Compute-a432100/sgiulitti@cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
				Parentpool: "/oracle/public/ippool",
				Permanent:  false,
				Quota:      nil,
				Tags:       []string{},
				Uri:        "https://compute.uscom-central-1.oraclecloud.com/ip/reservation/Compute-a432100/sgiulitti%40cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
				Used:       true,
			},
			Create: response.IpReservation{
				Account:    "/Compute-a432100/default",
				Ip:         "129.150.66.1",
				Name:       "/Compute-a432100/sgiulitti@cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
				Parentpool: "/oracle/public/ippool",
				Permanent:  false,
				Quota:      nil,
				Tags:       []string{},
				Uri:        "https://compute.uscom-central-1.oraclecloud.com/ip/reservation/Compute-a432100/sgiulitti%40cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
				Used:       true,
			},
			UpdateErr: nil,
			CreateErr: nil,
			AllErr:    nil,
			DeleteErr: nil,
		},
		FakeIpAssociation: FakeIpAssociation{
			All: response.AllIpAssociations{
				Result: []response.IpAssociation{
					response.IpAssociation{
						Account:     "/Compute-a432100/default",
						Ip:          "129.150.66.1",
						Name:        "/Compute-a432100/sgiulitti@cloudbase.com/f490e701-c68f-415d-a166-b75f1e1116d4",
						Parentpool:  "ippool:/oracle/public/ippool",
						Reservation: "/Compute-a432100/sgiulitti@cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
						Uri:         "https://compute.uscom-central-1.oraclecloud.com/ip/association/Compute-a432100/sgiulitti%40cloudbase.com/f490e701-c68f-415d-a166-b75f1e1116d4",
						Vcable:      "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6",
					},
				},
			},
			AllErr: nil,
			Create: response.IpAssociation{
				Account:     "/Compute-a432100/default",
				Ip:          "129.150.66.1",
				Name:        "/Compute-a432100/sgiulitti@cloudbase.com/f490e701-c68f-415d-a166-b75f1e1116d4",
				Parentpool:  "ippool:/oracle/public/ippool",
				Reservation: "/Compute-a432100/sgiulitti@cloudbase.com/29286b89-8d91-45b7-8a3a-78fffa2109b9",
				Uri:         "https://compute.uscom-central-1.oraclecloud.com/ip/association/Compute-a432100/sgiulitti%40cloudbase.com/f490e701-c68f-415d-a166-b75f1e1116d4",
				Vcable:      "/Compute-a432100/sgiulitti@cloudbase.com/faa46f2e-28c9-4500-b060-0997717540a6",
			},
			CreateErr: nil,
			DeleteErr: nil,
		},
		FakeIpNetworkExchanger: FakeIpNetworkExchanger{
			All: response.AllIpNetworkExchanges{
				Result: []response.IpNetworkExchange{
					response.IpNetworkExchange{
						Description: nil,
						Name:        "/Compute-a432100/sgiulitti@cloudbase.com/dbsql",
						Tags:        []string{},
						Uri:         "https://compute.uscom-central-1.oraclecloud.com:443/network/v1/ipnetworkexchange/Compute-a432100/sgiulitti@cloudbase.com/dbsql",
					},
					response.IpNetworkExchange{
						Description: nil,
						Name:        "/Compute-a432100/sgiulitti@cloudbase.com/test",
						Tags:        []string{},
						Uri:         "https://compute.uscom-central-1.oraclecloud.com:443/network/v1/ipnetworkexchange/Compute-a432100/sgiulitti@cloudbase.com/test",
					},
				},
			},
			AllErr: nil,
		},
		FakeIpNetworker: FakeIpNetworker{
			All: response.AllIpNetworks{
				Result: []response.IpNetwork{
					response.IpNetwork{
						Description:       nil,
						IpAddressPrefix:   "120.120.120.0/24",
						IpNetworkExchange: nil,
						Name:              "/Compute-a432100/sgiulitti@cloudbase.com/juju-charm-db",
						PublicNaptEnabledFlag: false,
						Tags: []string{},
						Uri:  "https://compute.uscom-central-1.oraclecloud.com:443/network/v1/ipnetwork/Compute-a432100/sgiulitti@cloudbase.com/juju-charm-db",
					},
				},
			},
			AllErr: nil,
		},
		FakeVnicSet: FakeVnicSet{
			VnicSetErr: nil,
			CreateErr:  nil,
			DeleteErr:  nil,
		},
		FakeStorageAPI:  *DefaultFakeStorageAPI,
		FakeRules:       DefaultFakeRules,
		FakeApplication: DefaultSecApplications,
		FakeSecIp:       DefaultSecIp,
		FakeSecList:     DefaultFakeSecList,
		FakeAssociation: DefaultFakeAssociation,
		FakeAcl:         DefaultFakeAcl,
		FakeSecRules:    DefaultFakeSecrules,
	}
)
