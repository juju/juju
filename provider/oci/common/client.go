// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"crypto/rsa"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	ociCommon "github.com/oracle/oci-go-sdk/common"
	ociCore "github.com/oracle/oci-go-sdk/core"
	ociIdentity "github.com/oracle/oci-go-sdk/identity"
)

type jujuConfigProvider struct {
	key            []byte
	keyFingerprint string
	passphrase     string
	tenancyOCID    string
	userOCID       string
	region         string
}

type ociClient struct {
	ociCore.ComputeClient
	ociCore.BlockstorageClient
	ociCore.VirtualNetworkClient
	ociIdentity.IdentityClient

	ociCommon.ConfigurationProvider
}

// NewJujuConfigProvider returns a new ociCommon.ConfigurationProvider instance
func NewJujuConfigProvider(user, tenant string, key []byte, fingerprint, passphrase, region string) ociCommon.ConfigurationProvider {
	return &jujuConfigProvider{
		key:            key,
		keyFingerprint: fingerprint,
		passphrase:     passphrase,
		tenancyOCID:    tenant,
		userOCID:       user,
		region:         region,
	}
}

func NewOciClient(provider ociCommon.ConfigurationProvider) (ApiClient, error) {
	computeClient, err := ociCore.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	blockStorage, err := ociCore.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	virtualNetwork, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ident, err := ociIdentity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ociClient{
		computeClient,
		blockStorage,
		virtualNetwork,
		ident,
		provider,
	}, nil
}

// Ping validates that the client can access the OCI API successfully
func (o *ociClient) Ping() error {
	tenancyID, err := o.TenancyOCID()
	if err != nil {
		return errors.Trace(err)
	}
	request := ociIdentity.ListCompartmentsRequest{
		CompartmentId: &tenancyID,
	}
	ctx := context.Background()
	_, err = o.ListCompartments(ctx, request)
	return err
}

func (o *ociClient) GetInstanceVnicAttachments(instanceID instance.Id, compartmentID *string) (ociCore.ListVnicAttachmentsResponse, error) {
	instID := string(instanceID)
	request := ociCore.ListVnicAttachmentsRequest{
		CompartmentId: compartmentID,
		InstanceId:    &instID,
	}
	response, err := o.ListVnicAttachments(context.Background(), request)
	if err != nil {
		return ociCore.ListVnicAttachmentsResponse{}, errors.Trace(err)
	}
	return response, nil
}

func (o *ociClient) GetInstanceVnics(vnics []ociCore.VnicAttachment) ([]ociCore.GetVnicResponse, error) {
	result := []ociCore.GetVnicResponse{}

	for _, val := range vnics {
		vnicID := val.VnicId
		request := ociCore.GetVnicRequest{
			VnicId: vnicID,
		}
		response, err := o.GetVnic(context.Background(), request)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, response)
	}
	return result, nil
}

func (o *ociClient) GetInstanceAddresses(instanceID instance.Id, compartmentID *string) ([]network.Address, error) {
	attachments, err := o.GetInstanceVnicAttachments(instanceID, compartmentID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	vnics, err := o.GetInstanceVnics(attachments.Items)
	if err != nil {
		return nil, errors.Trace(err)
	}

	addresses := []network.Address{}

	for _, val := range vnics {
		if val.Vnic.PrivateIp != nil {
			privateAddress := network.Address{
				Value: *val.Vnic.PrivateIp,
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			}
			addresses = append(addresses, privateAddress)
		}
		if val.Vnic.PublicIp != nil {
			publicAddress := network.Address{
				Value: *val.Vnic.PublicIp,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, publicAddress)
		}
	}
	return addresses, nil
}

func (j jujuConfigProvider) TenancyOCID() (string, error) {
	if j.tenancyOCID == "" {
		return "", errors.Errorf("tenancyOCID is not set")
	}
	return j.tenancyOCID, nil
}

func (j jujuConfigProvider) UserOCID() (string, error) {
	if j.userOCID == "" {
		return "", errors.Errorf("userOCID is not set")
	}
	return j.userOCID, nil
}

func (j jujuConfigProvider) KeyFingerprint() (string, error) {
	if j.keyFingerprint == "" {
		return "", errors.Errorf("keyFingerprint is not set")
	}
	return j.keyFingerprint, nil
}

func (j jujuConfigProvider) Region() (string, error) {
	return j.region, nil
}

func (j jujuConfigProvider) PrivateRSAKey() (*rsa.PrivateKey, error) {
	if j.key == nil {
		return nil, errors.Errorf("private key is not set")
	}

	key, err := ociCommon.PrivateKeyFromBytes(
		j.key, &j.passphrase)
	return key, err
}

func (j jujuConfigProvider) KeyID() (string, error) {
	if j.tenancyOCID == "" || j.userOCID == "" || j.keyFingerprint == "" {
		return "", errors.Errorf("config provider is not properly initialized")
	}
	return fmt.Sprintf("%s/%s/%s", j.tenancyOCID, j.userOCID, j.keyFingerprint), nil
}
