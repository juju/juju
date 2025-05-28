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

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	instances "github.com/juju/juju/environs/instances"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

type iaasAddressFinderSuite struct {
	instanceLister  *MockInstanceLister
	providerFactory *MockProviderFactory
}

func TestIAASAddressFinderSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &iaasAddressFinderSuite{})
}

func (s *iaasAddressFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceLister = NewMockInstanceLister(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)
	return ctrl
}

// TestIAASAddressFinderNoProvider is asserting that if there is no
// provider for the model that the error is returned.
func (s *iaasAddressFinderSuite) TestIAASAddressFinderNoProvider(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		nil, nil,
	)

	_, err := IAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorMatches, "cannot get instance lister from provider for finding bootstrap addresses.*")
}

// TestIAASAddressFinderProviderError is asserting that if getting a
// provider produces an error that error is maintained back up the stack.
func (s *iaasAddressFinderSuite) TestIAASAddressFinderProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	s.instanceLister.EXPECT().Instances(gomock.Any(), gomock.Any()).Return(
		nil, boom,
	)
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&iaasStubProvider{
			instanceLister: s.instanceLister,
		}, nil,
	)

	_, err := IAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorIs, boom)
}

// TestIAASAddressFinder is asserting the happy path of finding an instance
// addresses via a provider.
func (s *iaasAddressFinderSuite) TestIAASAddressFinder(c *tc.C) {
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
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&iaasStubProvider{
			instanceLister: s.instanceLister,
		}, nil,
	)

	foundAddresses, err := IAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundAddresses, tc.DeepEquals, addresses)
}

type caasAddressFinderSuite struct {
	serviceManager  *MockServiceManager
	providerFactory *MockProviderFactory
}

func TestCAASAddressFinderSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &caasAddressFinderSuite{})
}

func (s *caasAddressFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.serviceManager = NewMockServiceManager(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)
	return ctrl
}

// TestCAASAddressFinderNoProvider is asserting that if there is no
// provider for the model that the error is returned.
func (s *caasAddressFinderSuite) TestCAASAddressFinderNoProvider(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		nil, nil,
	)

	_, err := CAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorMatches, "cannot get service manager from provider for finding bootstrap addresses.*")
}

// TestCAASAddressFinderProviderError is asserting that if getting a
// provider produces an error that error is maintained back up the stack.
func (s *caasAddressFinderSuite) TestCAASAddressFinderProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	s.serviceManager.EXPECT().GetService(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, boom,
	)
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&caasStubProvider{
			serviceManager: s.serviceManager,
		}, nil,
	)

	_, err := CAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorIs, boom)
}

// TestCAASAddressFinder is asserting the happy path of finding an instance
// addresses via a provider.
func (s *caasAddressFinderSuite) TestCAASAddressFinder(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	addresses := network.NewMachineAddresses([]string{
		"2001:0DB8::BEEF:FACE",
	}).AsProviderAddresses()
	svc := &caas.Service{
		Addresses: addresses,
	}
	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(svc, nil)
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&caasStubProvider{
			serviceManager: s.serviceManager,
		}, nil,
	)

	foundAddresses, err := CAASAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundAddresses, tc.DeepEquals, addresses)
}

type iaasStubProvider struct {
	providertracker.Provider
	instanceLister environs.InstanceLister
}

// Instances implements environs.InstanceLister.
func (f *iaasStubProvider) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	return f.instanceLister.Instances(ctx, ids)
}

// GetService(ctx context.Context, appName string, includeClusterIP bool) (*Service, error)

type caasStubProvider struct {
	providertracker.Provider
	serviceManager caas.ServiceManager
}

// GetService implements caas.ServiceManager.
func (f *caasStubProvider) GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error) {
	return f.serviceManager.GetService(ctx, appName, includeClusterIP)
}
