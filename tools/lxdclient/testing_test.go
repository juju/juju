// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"runtime"

	"github.com/gorilla/websocket"
	containerlxd "github.com/juju/juju/container/lxd"
	"github.com/juju/testing"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite

	Stub   *testing.Stub
	Client *stubClient
	Cert   *containerlxd.Certificate
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
	s.Cert = &containerlxd.Certificate{
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

func (s *stubClient) CreateContainerFile(containerName string, path string, args lxd.ContainerFileArgs) error {
	s.stub.AddCall("CreateContainerFile", containerName, path, args)
	if err := s.stub.NextErr(); err != nil {
		return err
	}
	return nil
}

func (s *stubClient) CreateContainerFromImage(
	source lxd.ImageServer, image api.Image, imgcontainer api.ContainersPost,
) (lxd.RemoteOperation, error) {
	s.stub.AddCall("CreateContainerFromImage", source, image, imgcontainer)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *stubClient) DeleteContainer(name string) (lxd.Operation, error) {
	s.stub.AddCall("DeleteContainer", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.newOp(), nil
}

func (s *stubClient) GetContainers() ([]api.Container, error) {
	s.stub.AddCall("GetContainers")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.Instances, nil
}

func (s *stubClient) GetContainer(name string) (*api.Container, string, error) {
	s.stub.AddCall("GetContainer", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, "", err
	}
	return &api.Container{
		Name: name,
		ContainerPut: api.ContainerPut{
			Devices: map[string]map[string]string{},
		},
	}, "etag", nil
}

func (s *stubClient) GetContainerState(name string) (*api.ContainerState, string, error) {
	s.stub.AddCall("GetContainerState", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, "", err
	}
	return s.Instance, "etag", nil

}

func (s *stubClient) UpdateContainer(name string, container api.ContainerPut, ETag string) (lxd.Operation, error) {
	s.stub.AddCall("UpdateContainer", name, container, ETag)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.newOp(), nil
}

func (s *stubClient) UpdateContainerState(
	name string, container api.ContainerStatePut, ETag string,
) (lxd.Operation, error) {
	s.stub.AddCall("UpdateContainerState", name, container, ETag)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.newOp(), nil
}

type stubOp struct {
	stub *testing.Stub
}

func (o *stubOp) Wait() error {
	return o.stub.NextErr()
}

func (o *stubOp) Cancel() error {
	return nil
}

func (o *stubOp) Refresh() error {
	return nil
}

func (o *stubOp) Get() (op api.Operation) {
	return api.Operation{}
}

func (o *stubOp) GetWebsocket(secret string) (*websocket.Conn, error) {
	return nil, nil
}

func (o *stubOp) AddHandler(function func(api.Operation)) (target *lxd.EventTarget, err error) {
	return nil, nil
}

func (o *stubOp) RemoveHandler(target *lxd.EventTarget) error {
	return nil
}

func (s *stubClient) newOp() lxd.Operation {
	return &stubOp{s.stub}
}
