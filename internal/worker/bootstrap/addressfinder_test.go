// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	instances "github.com/juju/juju/environs/instances"
)

type addressFinderSuite struct {
	instanceLister *MockInstanceLister
}

var _ = gc.Suite(&addressFinderSuite{})

func (s *addressFinderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceLister = NewMockInstanceLister(ctrl)
	return ctrl
}

// TestBoostrapAddressFinderNotSupported is asserting that when a provider is
// requested that doesn't support the [environs.InstanceLister] we get back a
// default address set of "localhost".
func (*addressFinderSuite) TestBootstrapAddressFinderNotSupported(c *gc.C) {
	expected := network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses()

	addresses, err := BootstrapAddressFinder(
		func(_ context.Context) (environs.InstanceLister, error) {
			return nil, errors.NotSupported
		},
	)(context.Background(), instance.Id("12345"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(addresses, jc.DeepEquals, expected)
}

// TestBootstrapAddressFinderProviderError is asserting that if getting a
// provider produces an error that error is maintained back up the stack.
func (*addressFinderSuite) TestBootstrapAddressFinderProviderError(c *gc.C) {
	boom := errors.New("boom")
	_, err := BootstrapAddressFinder(
		func(_ context.Context) (environs.InstanceLister, error) {
			return nil, boom
		},
	)(context.Background(), instance.Id("12345"))
	c.Check(err, jc.ErrorIs, boom)
}

// TestBootstrapAddressFinder is asserting the happy path of finding an instance
// addresses via a provider.
func (s *addressFinderSuite) TestBoostrapAddressFinder(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	addresses := network.NewMachineAddresses([]string{
		"2001:0DB8::BEEF:FACE",
	}).AsProviderAddresses()

	inst := NewMockInstance(ctrl)
	inst.EXPECT().Addresses(gomock.Any()).Return(
		addresses, nil,
	)

	instId := instance.Id("12345")
	s.instanceLister.EXPECT().Instances(gomock.Any(), []instance.Id{instId}).Return(
		[]instances.Instance{inst}, nil,
	)

	foundAddresses, err := BootstrapAddressFinder(
		func(_ context.Context) (environs.InstanceLister, error) {
			return s.instanceLister, nil
		},
	)(context.Background(), instance.Id("12345"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(foundAddresses, jc.DeepEquals, addresses)
}
