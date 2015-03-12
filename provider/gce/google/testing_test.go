// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/google-api-go-client/compute/v1"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	Auth     Auth
	Conn     *Connection
	FakeConn *fakeConn

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

	s.Auth = Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	fake := &fakeConn{}
	s.Conn = &Connection{
		Region:    "a",
		ProjectID: "spam",
		raw:       fake,
	}
	s.FakeConn = fake

	s.DiskSpec = DiskSpec{
		SizeHintGB: 5,
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
			DiskSizeGb:  1,
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

func (s *BaseSuite) patchNewToken(c *gc.C, expectedAuth Auth, expectedScopes string, token *oauth.Token) {
	if expectedScopes == "" {
		expectedScopes = "https://www.googleapis.com/auth/compute https://www.googleapis.com/auth/devstorage.full_control"
	}
	if token == nil {
		token = &oauth.Token{}
	}
	s.PatchValue(&newToken, func(auth Auth, scopes string) (*oauth.Token, error) {
		c.Check(auth, jc.DeepEquals, expectedAuth)
		c.Check(scopes, gc.Equals, expectedScopes)
		return token, nil
	})
}

type fakeCall struct {
	FuncName string

	ProjectID string
	Region    string
	ZoneName  string
	Name      string
	ID        string
	Prefix    string
	Statuses  []string
	Instance  *compute.Instance
	InstValue compute.Instance
	Firewall  *compute.Firewall
}

type fakeConn struct {
	Calls []fakeCall

	Project    *compute.Project
	Instance   *compute.Instance
	Instances  []*compute.Instance
	Firewall   *compute.Firewall
	Zones      []*compute.Zone
	Err        error
	FailOnCall int
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
