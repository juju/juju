// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/proxy"
)

type dummyProxier struct {
	Conf string
}

type proxyWrapperSuite struct {
	factory *proxy.Factory
}

var _ = gc.Suite(&proxyWrapperSuite{})

func (d *dummyProxier) MarshalYAML() (interface{}, error) {
	if d.Conf == "" {
		d.Conf = "test"
	}
	return map[string]string{
		"conf": d.Conf,
	}, nil
}

func (d *dummyProxier) Start() error {
	return nil
}

func (d *dummyProxier) Stop() {
}

func (d *dummyProxier) Type() string {
	return "dummy-proxier"
}

func (p *proxyWrapperSuite) SetUpTest(c *gc.C) {
	p.factory = proxy.NewFactory()
}

func (p *proxyWrapperSuite) TestMarshallingKeys(c *gc.C) {
	proxier := &dummyProxier{}
	wrapper := jujuclient.ProxyConfWrapper{proxier}
	marshalled, err := wrapper.MarshalYAML()
	c.Assert(err, jc.ErrorIsNil)

	marshalledMap, valid := marshalled.(map[string]interface{})
	c.Assert(valid, jc.IsTrue)

	typeVal, valid := marshalledMap["type"].(string)
	c.Assert(valid, jc.IsTrue)
	c.Assert(typeVal, gc.Equals, proxier.Type())

	_, valid = marshalledMap["config"].(*dummyProxier)
	c.Assert(valid, jc.IsTrue)
}

func (p *proxyWrapperSuite) TestUnmarshalling(c *gc.C) {
	proxier := &dummyProxier{}
	wrapper := &jujuclient.ProxyConfWrapper{proxier}

	y, err := yaml.Marshal(wrapper)
	c.Assert(err, jc.ErrorIsNil)

	err = p.factory.Register(proxier.Type(), proxy.FactoryRegister{
		ConfigFn: func() interface{} {
			return &dummyProxier{}
		},
		MakerFn: func(i interface{}) (proxy.Proxier, error) {
			t, valid := i.(*dummyProxier)
			c.Assert(valid, jc.IsTrue)
			return t, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	fmt.Println(string(y))

	jujuclient.NewProxierFactory = func() (*proxy.Factory, error) {
		return p.factory, nil
	}

	inWrapper := &jujuclient.ProxyConfWrapper{}
	c.Assert(yaml.Unmarshal(y, inWrapper), jc.ErrorIsNil)
}
