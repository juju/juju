// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/google-api-go-client/compute/v1"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	auth             Auth
	DiskSpec         DiskSpec
	AttachedDisk     compute.AttachedDisk
	NetworkSpec      NetworkSpec
	NetworkInterface compute.NetworkInterface
	RawMetadata      compute.Metadata
	Metadata         map[string]string
	RawInstance      compute.Instance
	InstanceSpec     InstanceSpec
	Instance         Instance
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}

	s.DiskSpec = DiskSpec{
		SizeHintGB: 1,
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
		Zone:              "a-zone",
		NetworkInterfaces: []*compute.NetworkInterface{&s.NetworkInterface},
		Metadata:          &s.RawMetadata,
		Disks:             []*compute.AttachedDisk{&s.AttachedDisk},
		Tags:              &compute.Tags{Items: []string{"spam"}},
	}
	s.InstanceSpec = InstanceSpec{
		ID:                "spam",
		Type:              "sometype",
		Disks:             []DiskSpec{s.DiskSpec},
		Network:           s.NetworkSpec,
		NetworkInterfaces: []string{"somenetif"},
		Metadata:          s.Metadata,
		Tags:              []string{"spam"},
	}
	s.Instance = Instance{
		ID:   "spam",
		Zone: "a-zone",
		raw:  s.RawInstance,
		spec: &s.InstanceSpec,
	}
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
