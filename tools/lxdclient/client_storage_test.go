// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
)

type StorageClientSuite struct {
	testing.IsolationSuite

	raw *mockRawStorageClient
}

var _ = gc.Suite(&StorageClientSuite{})

func (s *StorageClientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.raw = &mockRawStorageClient{}
}

func (s *StorageClientSuite) TestStorageSupported(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	c.Assert(client.StorageSupported(), jc.IsTrue)
}

func (s *StorageClientSuite) TestStorageNotSupported(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, false)
	c.Assert(client.StorageSupported(), jc.IsFalse)

	err := client.VolumeCreate("pool", "volume", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	err = client.VolumeDelete("pool", "volume")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)

	_, err = client.VolumeList("pool")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *StorageClientSuite) TestVolumeCreate(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	cfg := map[string]string{"foo": "bar"}
	err := client.VolumeCreate("pool", "volume", cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCallNames(c, "StoragePoolVolumeTypeCreate")
	s.raw.CheckCall(c, 0, "StoragePoolVolumeTypeCreate", "pool", "volume", "custom", cfg)
}

func (s *StorageClientSuite) TestVolumeCreateError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.VolumeCreate("pool", "volume", nil)
	c.Assert(err, gc.ErrorMatches, "burp")
}

func (s *StorageClientSuite) TestVolumeDelete(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.VolumeDelete("pool", "volume")
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCallNames(c, "StoragePoolVolumeTypeDelete")
	s.raw.CheckCall(c, 0, "StoragePoolVolumeTypeDelete", "pool", "volume", "custom")
}

func (s *StorageClientSuite) TestVolumeDeleteError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.VolumeDelete("pool", "volume")
	c.Assert(err, gc.ErrorMatches, "burp")
}

func (s *StorageClientSuite) TestVolumeDeleteNotFound(c *gc.C) {
	s.raw.SetErrors(lxd.LXDErrors[http.StatusNotFound])
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.VolumeDelete("pool", "volume")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `volume "volume" in pool "pool" not found`)
}

func (s *StorageClientSuite) TestVolumeList(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	s.raw.volumes = []api.StorageVolume{{
		Type: "custom",
		StorageVolumePut: api.StorageVolumePut{
			Name: "foo",
		},
	}, {
		Type: "not-custom",
		StorageVolumePut: api.StorageVolumePut{
			Name: "bar",
		},
	}}
	list, err := client.VolumeList("pool")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list, jc.DeepEquals, s.raw.volumes[:1])
	s.raw.CheckCallNames(c, "StoragePoolVolumeTypeList")
	s.raw.CheckCall(c, 0, "StoragePoolVolumeTypeList", "pool")
}

func (s *StorageClientSuite) TestVolumeListError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.VolumeList("pool")
	c.Assert(err, gc.ErrorMatches, "burp")
}

func (s *StorageClientSuite) TestVolume(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	volume, err := client.Volume("pool", "volume")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Name, gc.Equals, "volume")
	s.raw.CheckCallNames(c, "StoragePoolVolumeTypeGet")
	s.raw.CheckCall(c, 0, "StoragePoolVolumeTypeGet", "pool", "volume", "custom")
}

func (s *StorageClientSuite) TestVolumeError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.Volume("pool", "volume")
	c.Assert(err, gc.ErrorMatches, "burp")
}

func (s *StorageClientSuite) TestVolumeUpdate(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	volume := api.StorageVolume{
		StorageVolumePut: api.StorageVolumePut{
			Name: "volume",
		},
	}
	err := client.VolumeUpdate("pool", "volume", volume)
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCallNames(c, "StoragePoolVolumeTypePut")
	s.raw.CheckCall(c, 0, "StoragePoolVolumeTypePut", "pool", "volume", "custom", volume)
}

func (s *StorageClientSuite) TestVolumeUpdateError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.VolumeUpdate("pool", "volume", api.StorageVolume{})
	c.Assert(err, gc.ErrorMatches, "burp")
}

func (s *StorageClientSuite) TestStoragePool(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.StoragePool("pool")
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCalls(c, []testing.StubCall{{"StoragePoolGet", []interface{}{"pool"}}})
}

func (s *StorageClientSuite) TestStoragePoolError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.StoragePool("pool")
	c.Assert(err, gc.ErrorMatches, `getting storage pool "pool": burp`)
}

func (s *StorageClientSuite) TestStoragePoolNotFound(c *gc.C) {
	s.raw.SetErrors(lxd.LXDErrors[http.StatusNotFound])
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.StoragePool("pool")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `storage pool "pool" not found`)
}

func (s *StorageClientSuite) TestStoragePools(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.StoragePools()
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCalls(c, []testing.StubCall{{"ListStoragePools", nil}})
}

func (s *StorageClientSuite) TestStoragePoolsError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	_, err := client.StoragePools()
	c.Assert(err, gc.ErrorMatches, "listing storage pools: burp")
}

func (s *StorageClientSuite) TestCreateStoragePool(c *gc.C) {
	client := lxdclient.NewStorageClient(s.raw, true)
	attrs := map[string]string{
		"a": "b",
		"c": "d",
	}
	err := client.CreateStoragePool("name", "driver", attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.raw.CheckCalls(c, []testing.StubCall{{
		"StoragePoolCreate",
		[]interface{}{"name", "driver", attrs},
	}})
}

func (s *StorageClientSuite) TestCreateStoragePoolError(c *gc.C) {
	s.raw.SetErrors(errors.New("burp"))
	client := lxdclient.NewStorageClient(s.raw, true)
	err := client.CreateStoragePool("name", "driver", nil)
	c.Assert(err, gc.ErrorMatches, `creating storage pool "name": burp`)
}

type mockRawStorageClient struct {
	testing.Stub
	volumes []api.StorageVolume
}

func (c *mockRawStorageClient) StoragePoolVolumeTypeCreate(pool string, volume string, volumeType string, config map[string]string) error {
	c.MethodCall(c, "StoragePoolVolumeTypeCreate", pool, volume, volumeType, config)
	return c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolVolumeTypeDelete(pool string, volume string, volumeType string) error {
	c.MethodCall(c, "StoragePoolVolumeTypeDelete", pool, volume, volumeType)
	return c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolVolumeTypeGet(pool, volume, volumeType string) (api.StorageVolume, error) {
	c.MethodCall(c, "StoragePoolVolumeTypeGet", pool, volume, volumeType)
	return api.StorageVolume{
		StorageVolumePut: api.StorageVolumePut{
			Name: volume,
		},
	}, c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolVolumeTypePut(pool, volume, volumeType string, update api.StorageVolume) error {
	c.MethodCall(c, "StoragePoolVolumeTypePut", pool, volume, volumeType, update)
	return c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolVolumesList(pool string) ([]api.StorageVolume, error) {
	c.MethodCall(c, "StoragePoolVolumeTypeList", pool)
	if err := c.NextErr(); err != nil {
		return nil, err
	}
	return c.volumes, nil
}

func (c *mockRawStorageClient) ListStoragePools() ([]api.StoragePool, error) {
	c.MethodCall(c, "ListStoragePools")
	return []api.StoragePool{}, c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolGet(name string) (api.StoragePool, error) {
	c.MethodCall(c, "StoragePoolGet", name)
	return api.StoragePool{}, c.NextErr()
}

func (c *mockRawStorageClient) StoragePoolCreate(name, driver string, attrs map[string]string) error {
	c.MethodCall(c, "StoragePoolCreate", name, driver, attrs)
	return c.NextErr()
}
