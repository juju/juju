// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	instances "github.com/juju/juju/environs/instances"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/testhelpers"
)

type iaasAddressFinderSuite struct {
	instanceLister  *MockInstanceLister
	providerFactory *MockProviderFactory
}

func TestIAASAddressFinderSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &iaasAddressFinderSuite{})
	})
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

type k8sAddressFinderSuite struct {
	serviceManager  *MockServiceManager
	providerFactory *MockProviderFactory
}

func TestK8sAddressFinderSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &k8sAddressFinderSuite{})
	})
}

func (s *k8sAddressFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.serviceManager = NewMockServiceManager(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)
	return ctrl
}

// TestK8sAddressFinderNoProvider is asserting that if there is no
// provider for the model that the error is returned.
func (s *k8sAddressFinderSuite) TestK8sAddressFinderNoProvider(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		nil, nil,
	)

	_, err := K8sAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorMatches, "cannot get service manager from provider for finding bootstrap addresses.*")
}

// TestK8sAddressFinderProviderError is asserting that if getting a
// provider produces an error that error is maintained back up the stack.
func (s *k8sAddressFinderSuite) TestK8sAddressFinderProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	s.serviceManager.EXPECT().GetService(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, boom,
	)
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&k8sStubProvider{
			serviceManager: s.serviceManager,
		}, nil,
	)

	_, err := K8sAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Check(err, tc.ErrorIs, boom)
}

// TestK8sAddressFinder is asserting the happy path of finding an instance
// addresses via a provider.
func (s *k8sAddressFinderSuite) TestK8sAddressFinder(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	addresses := network.NewMachineAddresses([]string{
		"2001:0DB8::BEEF:FACE",
		"api.example.com",
	}).AsProviderAddresses()
	svc := &caas.Service{
		Addresses: addresses,
	}
	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(svc, nil)
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&k8sStubProvider{
			serviceManager: s.serviceManager,
		}, nil,
	)

	foundAddresses, err := K8sAddressFinder(s.providerFactory, "controller-test")(c.Context(), instance.Id("12345"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundAddresses, tc.DeepEquals, addresses)
}

func (s *k8sAddressFinderSuite) TestK8sAddressFinderNoServiceAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "controller-test").Return(
		&k8sStubProvider{serviceManager: s.serviceManager}, nil,
	).AnyTimes()
	for _, svc := range []*caas.Service{nil, {}, {Id: "service-uid"}} {
		s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(svc, nil)
		_, err := K8sAddressFinder(s.providerFactory, "controller-test")(c.Context(), "controller-0")
		c.Assert(err, tc.ErrorMatches, "controller API service has no addresses")
	}
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

type k8sStubProvider struct {
	providertracker.Provider
	serviceManager caas.ServiceManager
}

// GetService implements caas.ServiceManager.
func (f *k8sStubProvider) GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error) {
	return f.serviceManager.GetService(ctx, appName, includeClusterIP)
}
