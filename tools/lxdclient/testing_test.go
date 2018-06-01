// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"crypto/x509"
	"io"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/testing"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite

	Stub   *testing.Stub
	Client *stubClient
	Cert   *lxd.Certificate
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	if runtime.GOOS == "windows" {
		c.Skip("LXD is not supported on Windows")
	}
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Stub = &testing.Stub{}
	s.Client = &stubClient{stub: s.Stub}
	s.Cert = &lxd.Certificate{
		Name:    "some cert",
		CertPEM: []byte("<a valid PEM-encoded x.509 cert>"),
		KeyPEM:  []byte("<a valid PEM-encoded x.509 key>"),
	}
}

type stubClient struct {
	stub *testing.Stub

	Instance   *api.ContainerState
	Instances  []api.Container
	ReturnCode int
	Response   *api.Response
	Aliases    map[string]string
}

func (s *stubClient) WaitForSuccess(waitURL string) error {
	s.stub.AddCall("WaitForSuccess", waitURL)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) SetServerConfig(key string, value string) (*api.Response, error) {
	s.stub.AddCall("SetServerConfig", key, value)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) CertificateAdd(cert *x509.Certificate, name string) error {
	s.stub.AddCall("CertificateAdd", cert, name)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) ContainerState(name string) (*api.ContainerState, error) {
	s.stub.AddCall("ContainerState", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instance, nil
}

func (s *stubClient) ListContainers() ([]api.Container, error) {
	s.stub.AddCall("ListContainers")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instances, nil
}

func (s *stubClient) GetAlias(alias string) string {
	s.stub.AddCall("GetAlias", alias)
	if err := s.stub.NextErr(); err != nil {
		return ""
	}
	return s.Aliases[alias]
}

func (s *stubClient) Init(name, remote, image string, profiles *[]string, config map[string]string, devices map[string]map[string]string, ephem bool) (*api.Response, error) {
	s.stub.AddCall("Init", name, remote, image, profiles, config, devices, ephem)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Delete(name string) (*api.Response, error) {
	s.stub.AddCall("Delete", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Action(name string, action shared.ContainerAction, timeout int, force bool, stateful bool) (*api.Response, error) {
	s.stub.AddCall("Action", name, action, timeout, force, stateful)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) SetContainerConfig(name, key, value string) error {
	s.stub.AddCall("SetContainerConfig", name, key, value)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) GetImageInfo(imageTarget string) (*api.Image, error) {
	s.stub.AddCall("GetImageInfo", imageTarget)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &api.Image{}, nil
}

func (s *stubClient) ContainerDeviceAdd(container, devname, devtype string, props []string) (*api.Response, error) {
	s.stub.AddCall("ContainerDeviceAdd", container, devname, devtype, props)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &api.Response{}, nil
}

func (s *stubClient) ContainerDeviceDelete(container, devname string) (*api.Response, error) {
	s.stub.AddCall("ContainerDeviceDelete", container, devname)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &api.Response{}, nil
}

func (s *stubClient) ContainerInfo(name string) (*api.Container, error) {
	s.stub.AddCall("ContainerInfo", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &api.Container{}, nil
}

func (s *stubClient) PushFile(container, path string, gid int, uid int, mode string, buf io.ReadSeeker) error {
	s.stub.AddCall("PushFile", container, path, gid, uid, mode, buf)
	if err := s.stub.NextErr(); err != nil {
		return err
	}
	return nil
}
