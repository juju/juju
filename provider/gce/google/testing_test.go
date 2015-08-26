// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	Credentials *Credentials
	ConnCfg     ConnectionConfig
	Conn        *Connection
	FakeConn    *fakeConn

	DiskSpec         DiskSpec
	AttachedDisk     compute.AttachedDisk
	NetworkSpec      NetworkSpec
	NetworkInterface compute.NetworkInterface
	Addresses        []network.Address
	RawMetadata      compute.Metadata
	Metadata         map[string]string
	RawInstance      compute.Instance
	RawInstanceFull  compute.Instance
	InstanceSpec     InstanceSpec
	Instance         Instance
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Credentials = &Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("<some-key>"),
		JSONKey: []byte(`
{
    "private_key_id": "mnopq",
    "private_key": "<some-key>",
    "client_email": "user@mail.com",
    "client_id": "spam",
    "type": "service_account"
}`[1:]),
	}
	s.ConnCfg = ConnectionConfig{
		Region:    "a",
		ProjectID: "spam",
	}
	fake := &fakeConn{}
	s.Conn = &Connection{
		raw:       fake,
		region:    "a",
		projectID: "spam",
	}
	s.FakeConn = fake

	s.DiskSpec = DiskSpec{
		SizeHintGB: 15,
		ImageURL:   "some/image/path",
		Boot:       true,
		Scratch:    false,
		Readonly:   false,
		AutoDelete: true,
	}
	s.AttachedDisk = compute.AttachedDisk{
		Type:       "PERSISTENT",
		Boot:       true,
		Mode:       "READ_WRITE",
		AutoDelete: true,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskSizeGb:  10,
			SourceImage: "some/image/path",
		},
	}
	s.NetworkSpec = NetworkSpec{
		Name: "somenetwork",
	}
	s.NetworkInterface = compute.NetworkInterface{
		Network:   "global/networks/somenetwork",
		NetworkIP: "10.0.0.1",
		AccessConfigs: []*compute.AccessConfig{{
			Name: "somenetif",
			Type: NetworkAccessOneToOneNAT,
		}},
	}
	s.Addresses = []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	s.RawMetadata = compute.Metadata{Items: []*compute.MetadataItems{{
		Key:   "eggs",
		Value: "steak",
	}}}
	s.Metadata = map[string]string{
		"eggs": "steak",
	}
	s.RawInstance = compute.Instance{
		Name:              "spam",
		Status:            StatusRunning,
		NetworkInterfaces: []*compute.NetworkInterface{&s.NetworkInterface},
		Metadata:          &s.RawMetadata,
		Disks:             []*compute.AttachedDisk{&s.AttachedDisk},
		Tags:              &compute.Tags{Items: []string{"spam"}},
	}
	s.RawInstanceFull = s.RawInstance
	s.RawInstanceFull.Zone = "a-zone"
	s.RawInstanceFull.Status = StatusRunning
	s.RawInstanceFull.MachineType = "zones/a-zone/machineTypes/mtype"
	s.InstanceSpec = InstanceSpec{
		ID:                "spam",
		Type:              "mtype",
		Disks:             []DiskSpec{s.DiskSpec},
		Network:           s.NetworkSpec,
		NetworkInterfaces: []string{"somenetif"},
		Metadata:          s.Metadata,
		Tags:              []string{"spam"},
	}
	s.Instance = Instance{
		InstanceSummary: InstanceSummary{
			ID:        "spam",
			ZoneName:  "a-zone",
			Status:    StatusRunning,
			Metadata:  s.Metadata,
			Addresses: s.Addresses,
		},
		spec: &s.InstanceSpec,
	}
}

func (s *BaseSuite) NewWaitError(op *compute.Operation, cause error) error {
	return waitError{op, cause}
}

type fakeCall struct {
	FuncName string

	ProjectID    string
	Region       string
	ZoneName     string
	Name         string
	ID           string
	Prefix       string
	Statuses     []string
	Instance     *compute.Instance
	InstValue    compute.Instance
	Firewall     *compute.Firewall
	InstanceId   string
	AttachedDisk *compute.AttachedDisk
	DeviceName   string
	ComputeDisk  *compute.Disk
}

type fakeConn struct {
	Calls []fakeCall

	Project       *compute.Project
	Instance      *compute.Instance
	Instances     []*compute.Instance
	Firewall      *compute.Firewall
	Zones         []*compute.Zone
	Err           error
	FailOnCall    int
	Disks         []*compute.Disk
	Disk          *compute.Disk
	AttachedDisks []*compute.AttachedDisk
}

func (rc *fakeConn) GetProject(projectID string) (*compute.Project, error) {
	call := fakeCall{
		FuncName:  "GetProject",
		ProjectID: projectID,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Project, err
}

func (rc *fakeConn) GetInstance(projectID, zone, id string) (*compute.Instance, error) {
	call := fakeCall{
		FuncName:  "GetInstance",
		ProjectID: projectID,
		ZoneName:  zone,
		ID:        id,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Instance, err
}

func (rc *fakeConn) ListInstances(projectID, prefix string, statuses ...string) ([]*compute.Instance, error) {
	call := fakeCall{
		FuncName:  "ListInstances",
		ProjectID: projectID,
		Prefix:    prefix,
		Statuses:  statuses,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Instances, err
}

func (rc *fakeConn) AddInstance(projectID, zoneName string, spec *compute.Instance) error {
	call := fakeCall{
		FuncName:  "AddInstance",
		ProjectID: projectID,
		ZoneName:  zoneName,
		Instance:  spec,
		InstValue: *spec,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) RemoveInstance(projectID, zone, id string) error {
	call := fakeCall{
		FuncName:  "RemoveInstance",
		ProjectID: projectID,
		ID:        id,
		ZoneName:  zone,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) GetFirewall(projectID, name string) (*compute.Firewall, error) {
	call := fakeCall{
		FuncName:  "GetFirewall",
		ProjectID: projectID,
		Name:      name,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Firewall, err
}

func (rc *fakeConn) AddFirewall(projectID string, firewall *compute.Firewall) error {
	call := fakeCall{
		FuncName:  "AddFirewall",
		ProjectID: projectID,
		Firewall:  firewall,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) UpdateFirewall(projectID, name string, firewall *compute.Firewall) error {
	call := fakeCall{
		FuncName:  "UpdateFirewall",
		ProjectID: projectID,
		Name:      name,
		Firewall:  firewall,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) RemoveFirewall(projectID, name string) error {
	call := fakeCall{
		FuncName:  "RemoveFirewall",
		ProjectID: projectID,
		Name:      name,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) ListAvailabilityZones(projectID, region string) ([]*compute.Zone, error) {
	call := fakeCall{
		FuncName:  "ListAvailabilityZones",
		ProjectID: projectID,
		Region:    region,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Zones, err
}

func (rc *fakeConn) CreateDisk(project, zone string, spec *compute.Disk) error {
	call := fakeCall{
		FuncName:    "CreateDisk",
		ProjectID:   project,
		ZoneName:    zone,
		ComputeDisk: spec,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) ListDisks(project, zone string) ([]*compute.Disk, error) {
	call := fakeCall{
		FuncName:  "ListDisks",
		ProjectID: project,
		ZoneName:  zone,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Disks, err
}

func (rc *fakeConn) RemoveDisk(project, zone, id string) error {
	call := fakeCall{
		FuncName:  "RemoveDisk",
		ProjectID: project,
		ZoneName:  zone,
		ID:        id,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) GetDisk(project, zone, id string) (*compute.Disk, error) {
	call := fakeCall{
		FuncName:  "GetDisk",
		ProjectID: project,
		ZoneName:  zone,
		ID:        id,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.Disk, err
}

func (rc *fakeConn) AttachDisk(project, zone, instanceId string, attachedDisk *compute.AttachedDisk) error {
	call := fakeCall{
		FuncName:     "AttachDisk",
		ProjectID:    project,
		ZoneName:     zone,
		InstanceId:   instanceId,
		AttachedDisk: attachedDisk,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) DetachDisk(project, zone, instanceId, diskDeviceName string) error {
	call := fakeCall{
		FuncName:   "DetachDisk",
		ProjectID:  project,
		ZoneName:   zone,
		InstanceId: instanceId,
		DeviceName: diskDeviceName,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return err
}

func (rc *fakeConn) InstanceDisks(project, zone, instanceId string) ([]*compute.AttachedDisk, error) {
	call := fakeCall{
		FuncName:   "InstanceDisks",
		ProjectID:  project,
		ZoneName:   zone,
		InstanceId: instanceId,
	}
	rc.Calls = append(rc.Calls, call)

	err := rc.Err
	if len(rc.Calls) != rc.FailOnCall+1 {
		err = nil
	}
	return rc.AttachedDisks, err
}
