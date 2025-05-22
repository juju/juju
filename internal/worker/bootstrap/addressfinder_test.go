// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	instances "github.com/juju/juju/environs/instances"
)

type addressFinderSuite struct {
	instanceLister *MockInstanceLister
}

func TestAddressFinderSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &addressFinderSuite{})
}

func (s *addressFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceLister = NewMockInstanceLister(ctrl)
	return ctrl
}

// TestBoostrapAddressFinderNotSupported is asserting that when a provider is
// requested that doesn't support the [environs.InstanceLister] we get back a
// default address set of "localhost".
func (*addressFinderSuite) TestBootstrapAddressFinderNotSupported(c *tc.C) {
	expected := network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses()

	addresses, err := BootstrapAddressFinder(
		func(_ context.Context) (environs.InstanceLister, error) {
			return nil, errors.NotSupported
		},
	)(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, expected)
}

// TestBootstrapAddressFinderProviderError is asserting that if getting a
// provider produces an error that error is maintained back up the stack.
func (*addressFinderSuite) TestBootstrapAddressFinderProviderError(c *tc.C) {
	boom := errors.New("boom")
	_, err := BootstrapAddressFinder(
		func(_ context.Context) (environs.InstanceLister, error) {
			return nil, boom
		},
	)(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorIs, boom)
}

// TestBootstrapAddressFinder is asserting the happy path of finding an instance
// addresses via a provider.
func (s *addressFinderSuite) TestBoostrapAddressFinder(c *tc.C) {
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
	)(c.Context(), instance.Id("12345"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundAddresses, tc.DeepEquals, addresses)
}
