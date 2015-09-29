// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	testing.BaseSuite
	mockAPI *mockListAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.setupBaseSuite(c)

	s.mockAPI = &mockListAPI{}
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		return testData, nil
	}
	s.PatchValue(&getImageMetadataListAPI, func(c *ListImagesCommand) (MetadataListAPI, error) {
		return s.mockAPI, nil
	})
}

func runList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&ListImagesCommand{}), args...)
}

func (s *ListSuite) TestListDefault(c *gc.C) {
	// Default format is tabular
	s.assertValidList(c, `
SOURCE  SERIES  ARCH   REGION  IMAGE_ID  STREAM    VIRT_TYPE  STORAGE_TYPE
custom  vivid   amd64  asia    im-21     released  kvm        ebs
custom  vivid   amd64  europe  im-21     released  kvm        ebs
custom  vivid   amd64  us      im-21     released  kvm        ebs
custom  trusty  amd64  europe  im-21     released  kvm        ebs
custom  trusty  i386   asia    im-21     released  kvm        ebs
custom  trusty  i386   europe  im-21     released  kvm        ebs
public  vivid   amd64  europe  im-21     released  kvm        ebs
public  trusty  i386   europe  im-21     released  kvm        ebs
public  trusty  i386   europe  im-42     devel     kvm        ebs

`[1:], "")
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(c, `
custom:
  trusty:
    amd64:
      europe:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
    i386:
      asia:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
      europe:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
  vivid:
    amd64:
      asia:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
      europe:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
      us:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
public:
  trusty:
    i386:
      europe:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
      - image_id: im-42
        stream: devel
        virt_type: kvm
        storage_type: ebs
  vivid:
    amd64:
      europe:
      - image_id: im-21
        stream: released
        virt_type: kvm
        storage_type: ebs
`[1:], "", "--format", "yaml")
}

func (s *ListSuite) TestListMetadataFailed(c *gc.C) {
	msg := "failed"
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		return nil, errors.New(msg)
	}

	_, err := runList(c, nil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
}

func (s *ListSuite) TestListMetadataFilterStream(c *gc.C) {
	msg := "stream"
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(stream, gc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--stream", msg)
}

func (s *ListSuite) TestListMetadataFilterRegion(c *gc.C) {
	msg := "region"
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(region, gc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--region", msg)
}

func (s *ListSuite) TestListMetadataFilterSeries(c *gc.C) {
	all := []string{"series1", "series2"}
	msg := strings.Join(all, ",")
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(ser, gc.DeepEquals, all)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--series", msg)
}

func (s *ListSuite) TestListMetadataFilterArches(c *gc.C) {
	all := []string{"arch1", "barch2"}
	msg := strings.Join(all, ",")
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(arch, gc.DeepEquals, all)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--arch", msg)
}

func (s *ListSuite) TestListMetadataFilterVirtType(c *gc.C) {
	msg := "virtType"
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(virtType, gc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--virtType", msg)
}

func (s *ListSuite) TestListMetadataFilterStorageType(c *gc.C) {
	msg := "storagetype"
	s.mockAPI.list = func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
		c.Assert(rootStorageType, gc.DeepEquals, msg)
		return nil, nil
	}
	s.assertValidList(c, "", "", "--storageType", msg)
}

func (s *ListSuite) assertValidList(c *gc.C, expectedValid, expectedErr string, args ...string) {
	context, err := runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Matches, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)
}

type mockListAPI struct {
	list func(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error)
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) List(stream, region string, ser, arch []string, virtType, rootStorageType string) ([]params.CloudImageMetadata, error) {
	return s.list(stream, region, ser, arch, virtType, rootStorageType)
}

func (s *ListSuite) setupBaseSuite(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	os.Setenv(osenv.JujuEnvEnvKey, "testing")
	info := memstore.CreateInfo("testing")
	info.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"127.0.0.1:12345"},
		Hostnames:   []string{"localhost:12345"},
		CACert:      testing.CACert,
		EnvironUUID: "env-uuid",
	})
	info.SetAPICredentials(configstore.APICredentials{
		User:     "user-test",
		Password: "password",
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

}

var testData = []params.CloudImageMetadata{
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "vivid",
		Arch:            "amd64",
		Region:          "asia",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "vivid",
		Arch:            "amd64",
		Region:          "us",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "vivid",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "public",
		Series:          "vivid",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "trusty",
		Arch:            "amd64",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "trusty",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "custom",
		Series:          "trusty",
		Arch:            "i386",
		Region:          "asia",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "public",
		Series:          "trusty",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-21",
		Stream:          "released",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
	params.CloudImageMetadata{
		Source:          "public",
		Series:          "trusty",
		Arch:            "i386",
		Region:          "europe",
		ImageId:         "im-42",
		Stream:          "devel",
		VirtType:        "kvm",
		RootStorageType: "ebs",
	},
}
