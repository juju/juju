// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"fmt"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type BaseCloudImageMetadataSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (s *BaseCloudImageMetadataSuite) SetUpTest(c *tc.C) {
	s.setupBaseSuite(c)
}

func (s *BaseCloudImageMetadataSuite) setupBaseSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {
				ModelType: model.IAAS,
			},
		},
		CurrentModel: "admin/controller",
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
}

type ListSuite struct {
	BaseCloudImageMetadataSuite
	mockAPI *mockListAPI
}

func TestListSuite(t *stdtesting.T) { tc.Run(t, &ListSuite{}) }
func (s *ListSuite) SetUpTest(c *tc.C) {
	s.BaseCloudImageMetadataSuite.SetUpTest(c)

	s.mockAPI = &mockListAPI{}
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		return testData, nil
	}
	s.PatchValue(&getImageMetadataListAPI, func(c *listImagesCommand, ctx context.Context) (MetadataListAPI, error) {
		return s.mockAPI, nil
	})
}

func runList(c *tc.C, args []string) (*cmd.Context, error) {
	cmd := &listImagesCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmdtesting.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

func (s *ListSuite) TestListDefault(c *tc.C) {
	// Default format is tabular
	s.assertValidList(c, `
Source  Version  Arch   Region  Image ID  Stream    Virt Type  Storage Type
custom  22.04    amd64  europe  im-21     released  kvm        ebs
custom  22.04    i386   asia    im-21     released  kvm        ebs
custom  22.04    i386   europe  im-21     released  kvm        ebs
custom  20.04    amd64  asia    im-21     released  kvm        ebs
custom  20.04    amd64  europe  im-21     released  kvm        ebs
custom  20.04    amd64  us      im-21     released  kvm        ebs
public  22.04    i386   europe  im-21     released  kvm        ebs
public  22.04    i386   europe  im-42     devel     kvm        ebs
public  22.04    i386   europe  im-42     devel                ebs
public  22.04    i386   europe  im-42     devel     kvm        
public  22.04    i386   europe  im-42     devel                
public  20.04    amd64  europe  im-21     released  kvm        ebs
`[1:], "")
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	s.assertValidList(c, `
custom:
  "20.04":
    amd64:
      asia:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
      europe:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
      us:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
  "22.04":
    amd64:
      europe:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
    i386:
      asia:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
      europe:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
public:
  "20.04":
    amd64:
      europe:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
  "22.04":
    i386:
      europe:
      - image-id: im-21
        stream: released
        virt-type: kvm
        storage-type: ebs
      - image-id: im-42
        stream: devel
        virt-type: kvm
        storage-type: ebs
      - image-id: im-42
        stream: devel
        storage-type: ebs
      - image-id: im-42
        stream: devel
        virt-type: kvm
      - image-id: im-42
        stream: devel
`[1:], "", "--format", "yaml")
}

func (s *ListSuite) TestListMetadataFailed(c *tc.C) {
	msg := "failed"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		return nil, errors.New(msg)
	}

	_, err := runList(c, nil)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListMetadataFilterStream(c *tc.C) {
	msg := "stream"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(stream, tc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--stream", msg)
}

func (s *ListSuite) TestListMetadataFilterRegion(c *tc.C) {
	msg := "region"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(region, tc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--region", msg)
}

func (s *ListSuite) TestListMetadataFilterBases(c *tc.C) {
	all := []string{"ubuntu@22.04", "ubuntu@20.04"}
	msg := strings.Join(all, ",")
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		expected := make([]corebase.Base, len(all))
		for i, b := range all {
			expected[i] = corebase.MustParseBaseFromString(b)
		}
		c.Assert(bases, tc.DeepEquals, expected)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--bases", msg)
}

func (s *ListSuite) TestListMetadataFilterArches(c *tc.C) {
	all := []string{"arch1", "barch2"}
	msg := strings.Join(all, ",")
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(arch, tc.DeepEquals, all)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--arch", msg)
}

func (s *ListSuite) TestListMetadataFilterVirtType(c *tc.C) {
	msg := "virtType"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(virtType, tc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--virt-type", msg)
}

func (s *ListSuite) TestListMetadataFilterStorageType(c *tc.C) {
	msg := "storagetype"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(rootStorageType, tc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--storage-type", msg)
}

func (s *ListSuite) TestListMetadataNoFilter(c *tc.C) {
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(rootStorageType, tc.DeepEquals, "")
		c.Assert(virtType, tc.DeepEquals, "")
		c.Assert(region, tc.DeepEquals, "")
		c.Assert(stream, tc.DeepEquals, "")
		c.Assert(bases, tc.IsNil)
		c.Assert(arch, tc.IsNil)
		return nil, nil
	}
	s.assertValidList(c, "", "")
}

func (s *ListSuite) TestListMetadataFewFilters(c *tc.C) {
	streamValue := "streamValue"
	regionValue := "regionValue"
	typeValue := "typeValue"
	s.mockAPI.list = func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(stream, tc.DeepEquals, streamValue)
		c.Assert(region, tc.DeepEquals, regionValue)
		c.Assert(virtType, tc.DeepEquals, typeValue)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--stream", streamValue, "--region", regionValue, "--virt-type", typeValue)
}

func (s *ListSuite) assertValidList(c *tc.C, expectedValid, expectedErr string, args ...string) {
	context, err := runList(c, args)
	c.Assert(err, tc.ErrorIsNil)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Matches, expectedErr)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, tc.Matches, expectedValid)
}

type mockListAPI struct {
	list func(stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) List(ctx context.Context, stream, region string, bases []corebase.Base, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
	return s.list(stream, region, bases, arch, virtType, rootStorageType)
}

var testData = []params.CloudImageMetadata{
	{
		Source:          "custom",
		Version:         "20.04",
		Arch:            "amd64",
		Region:          "asia",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "custom",
		Version:         "20.04",
		Arch:            "amd64",
		Region:          "us",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "custom",
		Version:         "20.04",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "public",
		Version:         "20.04",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "custom",
		Version:         "22.04",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "custom",
		Version:         "22.04",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "custom",
		Version:         "22.04",
		Arch:            "i386",
		Region:          "asia",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "public",
		Version:         "22.04",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "public",
		Version:         "22.04",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-42",
		Stream:          "devel",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	{
		Source:          "public",
		Version:         "22.04",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-42",
		Stream:          "devel",
		RootStorageType: "ebs",
	},
	{
		Source:   "public",
		Version:  "22.04",
		Arch:     "i386",
		Region:   "europe",
		ImageId:  "im-42",
		Stream:   "devel",
		VirtType: "kvm",
	},
	{
		Source:  "public",
		Version: "22.04",
		Arch:    "i386",
		Region:  "europe",
		ImageId: "im-42",
		Stream:  "devel",
	},
}
