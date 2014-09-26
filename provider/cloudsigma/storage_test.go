// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/testing"
	tt "github.com/juju/juju/testing"
)

type StorageSuite struct {
	tt.BaseSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestStorageEmpty(c *gc.C) {
	stg := &environStorage{}

	lst, err := stg.List("")
	c.Check(lst, gc.IsNil)
	c.Check(err, gc.NotNil)

	url, err := stg.URL("")
	c.Check(url, gc.Equals, "")
	c.Check(err, gc.NotNil)

	r, err := stg.Get("")
	c.Check(r, gc.IsNil)
	c.Check(err, gc.NotNil)

	err = stg.Put("", nil, 0)
	c.Check(err, gc.NotNil)

	err = stg.Remove("")
	c.Check(err, gc.NotNil)

	err = stg.RemoveAll()
	c.Check(err, gc.IsNil)

	strategy := stg.DefaultConsistencyStrategy()
	c.Check(strategy, gc.Equals, utils.AttemptStrategy{})

	shr := stg.ShouldRetry(nil)
	c.Check(shr, gc.Equals, false)

	err = stg.MoveToSSH("", "")
	c.Check(err, gc.NotNil)
}

func (s *StorageSuite) TestStorageInstanceStop(c *gc.C) {
	stg := &environStorage{
		storage: &fakeStorage{},
		uuid:    "uuid",
		tmp:     true,
	}
	stg.onStateInstanceStop("")
	c.Check(stg.uuid, gc.Equals, "uuid")
	stg.onStateInstanceStop("uuid")
	c.Check(stg.storage, gc.IsNil)
	c.Check(stg.uuid, gc.Equals, "")
	c.Check(stg.tmp, gc.Equals, false)
}

func (s *StorageSuite) TestStorageURL(c *gc.C) {
	fs := &fakeStorage{}
	stg := &environStorage{
		storage: fs,
		uuid:    "uuid",
		tmp:     true,
	}

	url, err := stg.URL("")
	c.Check(err, gc.IsNil)
	c.Check(url, gc.Equals, "%s/")

	url, err = stg.URL("path/name")
	c.Check(err, gc.IsNil)
	c.Check(url, gc.Equals, "%s/path/name")

	stg.tmp = false

	url, err = stg.URL("path/name")
	c.Check(err, gc.IsNil)
	c.Check(url, gc.Equals, "")
	c.Check(fs.call, gc.Equals, "URL")
	c.Check(fs.name, gc.Equals, "path/name")
}

func (s *StorageSuite) TestStorageProxy(c *gc.C) {
	fs := &fakeStorage{}
	stg := &environStorage{
		storage: fs,
		uuid:    "uuid",
		tmp:     true,
	}

	test := func(s storage.Storage) {

		ll := logger.LogLevel()
		logger.SetLogLevel(loggo.TRACE)
		s.List("list")
		c.Check(fs.call, gc.Equals, "List")
		c.Check(fs.prefix, gc.Equals, "list")
		logger.SetLogLevel(ll)

		s.Get("get")
		c.Check(fs.call, gc.Equals, "Get")
		c.Check(fs.name, gc.Equals, "get")

		r := strings.NewReader("")
		s.Put("put", r, 1024)
		c.Check(fs.call, gc.Equals, "Put")
		c.Check(fs.name, gc.Equals, "put")
		c.Check(fs.reader, gc.Equals, r)
		c.Check(fs.length, gc.Equals, int64(1024))

		s.Remove("remove")
		c.Check(fs.call, gc.Equals, "Remove")
		c.Check(fs.name, gc.Equals, "remove")

		s.RemoveAll()
		c.Check(fs.call, gc.Equals, "RemoveAll")

		s.DefaultConsistencyStrategy()
		c.Check(fs.call, gc.Equals, "DefaultConsistencyStrategy")

		err := fmt.Errorf("test")
		s.ShouldRetry(err)
		c.Check(fs.call, gc.Equals, "ShouldRetry")
		c.Check(fs.err, gc.Equals, err)
	}

	test(stg)

	stg.tmp = false
	test(stg)
}

func (s *StorageSuite) TestStorageMoveNonTemp(c *gc.C) {
	fs := &fakeStorage{}
	stg := &environStorage{
		storage: fs,
		uuid:    "uuid",
		tmp:     false,
	}

	err := stg.MoveToSSH("", "")
	c.Check(err, gc.NotNil)
}

func (s *StorageSuite) TestStorageMoveFailSsh(c *gc.C) {
	fs := &fakeStorage{}
	stg := &environStorage{
		storage: fs,
		uuid:    "uuid",
		tmp:     true,
	}

	s.PatchValue(&newSSHStorage,
		func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			return nil, fmt.Errorf("test")
		})

	err := stg.MoveToSSH("", "")
	c.Check(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "initializing SSH storage failed: test")
}

func (s *StorageSuite) TestStorageMoveFailList(c *gc.C) {
	fs := &fakeStorage{}
	stg := &environStorage{
		storage: fs,
		uuid:    "uuid",
		tmp:     true,
	}

	s.PatchValue(&newSSHStorage,
		func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			stg.storage = nil
			return nil, nil
		})

	err := stg.MoveToSSH("", "")
	c.Check(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "listing tmp storage failed: storage is not initialized")
}

func newTestStorage(s *StorageSuite, c *gc.C) storage.Storage {
	closer, stor, _ := testing.CreateLocalTestStorage(c)
	s.AddCleanup(func(*gc.C) { closer.Close() })
	return stor
}

type moveStorageTest struct {
	stg     storage.Storage
	handler func(_, _, _ string) (storage.Storage, error)
	data    map[string]string
	emsg    string
}

func (s *StorageSuite) testMoveStorage(c *gc.C, d *moveStorageTest) {

	s.PatchValue(&newSSHStorage, d.handler)

	for k, v := range d.data {
		err := d.stg.Put(k, strings.NewReader(v), int64(len(v)))
		c.Assert(err, gc.IsNil)
	}

	src := &environStorage{
		storage: d.stg,
		uuid:    "uuid",
		tmp:     true,
	}

	err := src.MoveToSSH("user", "host")
	if d.emsg == "" {
		c.Check(err, gc.IsNil)
	} else {
		c.Check(err, gc.ErrorMatches, d.emsg)
		return
	}

	kk, err := src.storage.List("")
	c.Assert(err, gc.IsNil)

	var result = make(map[string]string, len(kk))
	for _, k := range kk {
		r, err := src.storage.Get(k)
		c.Assert(err, gc.IsNil)
		defer r.Close()
		bb, err := ioutil.ReadAll(r)
		c.Assert(err, gc.IsNil)
		result[k] = string(bb)
	}

	for k, v := range d.data {
		if vv, ok := result[k]; !ok {
			c.Errorf("key %s not found", k)
		} else {
			c.Check(vv, gc.Equals, v)
		}
	}
}

func (s *StorageSuite) TestStorageMoveEmpty(c *gc.C) {
	data := &moveStorageTest{
		stg: newTestStorage(s, c),
		handler: func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			c.Check(sshurl, gc.Equals, "user@host")
			return newTestStorage(s, c), nil
		}}
	s.testMoveStorage(c, data)
}

var moveData = map[string]string{
	"test0":      "0987654321",
	"test/test1": "1234567890",
}

func (s *StorageSuite) TestStorageMoveData(c *gc.C) {
	data := &moveStorageTest{
		stg: newTestStorage(s, c),
		handler: func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			c.Check(sshurl, gc.Equals, "user@host")
			return newTestStorage(s, c), nil
		},
		data: moveData,
	}
	s.testMoveStorage(c, data)
}

type storageProxyGetFailed struct {
	storage.Storage
}

func (s *storageProxyGetFailed) Get(name string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("test")
}

func (s *StorageSuite) TestStorageMoveGetFailed(c *gc.C) {
	data := &moveStorageTest{
		stg: &storageProxyGetFailed{newTestStorage(s, c)},
		handler: func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			c.Check(sshurl, gc.Equals, "user@host")
			return newTestStorage(s, c), nil
		},
		data: moveData,
		emsg: "getting .* from tmp storage failed: test",
	}
	s.testMoveStorage(c, data)
}

type storageProxyGetReadFailed struct {
	storage.Storage
}

func (s *storageProxyGetReadFailed) Get(name string) (io.ReadCloser, error) {
	err := fmt.Errorf("test")
	r := &failReader{err}
	return ioutil.NopCloser(r), nil
}

func (s *StorageSuite) TestStorageMoveGetReadFailed(c *gc.C) {
	data := &moveStorageTest{
		stg: &storageProxyGetReadFailed{newTestStorage(s, c)},
		handler: func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			c.Check(sshurl, gc.Equals, "user@host")
			return newTestStorage(s, c), nil
		},
		data: moveData,
		emsg: ".*MoveToSSH: reading .* from ssh storage failed: test",
	}
	s.testMoveStorage(c, data)
}

type storageProxyPutFailed struct {
	storage.Storage
}

func (s *storageProxyPutFailed) Put(name string, r io.Reader, length int64) error {
	return fmt.Errorf("test")
}

func (s *StorageSuite) TestStorageMovePutFailed(c *gc.C) {
	data := &moveStorageTest{
		stg: newTestStorage(s, c),
		handler: func(sshurl, dir, tmpdir string) (storage.Storage, error) {
			c.Check(sshurl, gc.Equals, "user@host")
			return &storageProxyPutFailed{newTestStorage(s, c)}, nil
		},
		data: moveData,
		emsg: ".*MoveToSSH: putting .* to ssh storage failed: test",
	}
	s.testMoveStorage(c, data)
}

func (s *StorageSuite) TestStorageNewRemoteStorage(c *gc.C) {
	ecfg := &environConfig{
		Config: newConfig(c, tt.Attrs{
			"name": "client-test",
		}),
		attrs: map[string]interface{}{
			"storage-port": 1234,
		},
	}
	stg, err := newRemoteStorage(ecfg, "0.1.2.3")
	c.Check(stg, gc.NotNil)
	c.Check(err, gc.IsNil)

	attrs := tt.FakeConfig().Delete("ca-cert", "ca-private-key")
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	ecfg = &environConfig{
		Config: cfg,
		attrs: map[string]interface{}{
			"storage-port": 1234,
		}}
	stg, err = newRemoteStorage(ecfg, "0.1.2.3")
	c.Check(stg, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "ca-cert not set")
}
