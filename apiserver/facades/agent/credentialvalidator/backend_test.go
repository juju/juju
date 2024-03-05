// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"

	"github.com/juju/names/v5"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/state"
)

// modelUUID is the model tag we're using in the tests.
var modelUUID = "01234567-89ab-cdef-0123-456789abcdef"

// credentialTag is the credential tag we're using in the tests.
// needs to fit fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName)
var credentialTag = names.NewCloudCredentialTag("cloud/user/credential")

func newMockBackend() *testBackend {
	b := &testBackend{
		Stub:             &testing.Stub{},
		credentialExists: true,
		credentialTag:    credentialTag,
		credentialSet:    true,
	}
	return b
}

type testBackend struct {
	*testing.Stub

	credentialExists bool
	credentialTag    names.CloudCredentialTag
	credentialSet    bool
}

func (m *testBackend) CloudCredentialTag() (names.CloudCredentialTag, bool, error) {
	m.MethodCall(m, "mockModel.CloudCredentialTag")
	return m.credentialTag, m.credentialSet, nil
}

func (b *testBackend) Model() (credentialvalidator.ModelAccessor, error) {
	return &mockModel{
		Stub:     b.Stub,
		modelTag: names.NewModelTag(modelUUID),
	}, b.NextErr()
}

func (b *testBackend) Cloud(name string) (cloud.Cloud, error) {
	return cloud.Cloud{
		Name:      name,
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, nil
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}

func (b *testBackend) WatchModelCredential() (state.NotifyWatcher, error) {
	b.AddCall("WatchModelCredential")
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	return apiservertesting.NewFakeNotifyWatcher(), nil
}

func newMockCloudService() *testCloudService {
	s := &testCloudService{
		Stub: &testing.Stub{},
	}
	return s
}

type testCloudService struct {
	common.CloudService
	*testing.Stub
}

func (c testCloudService) Get(ctx context.Context, name string) (*cloud.Cloud, error) {
	return &cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, nil
}

func newMockCredentialService() *testCredentialService {
	s := &testCredentialService{
		Stub: &testing.Stub{},
	}
	return s
}

type testCredentialService struct {
	common.CredentialService
	*testing.Stub
}

func (c testCredentialService) InvalidateCredential(ctx context.Context, id credential.ID, reason string) error {
	c.AddCall("InvalidateCredential", id, reason)
	return c.NextErr()
}

func (c testCredentialService) CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error) {
	return cloud.NewEmptyCredential(), nil
}

func (c testCredentialService) WatchCredential(ctx context.Context, id credential.ID) (watcher.NotifyWatcher, error) {
	return apiservertesting.NewFakeNotifyWatcher(), nil
}

type mockModel struct {
	*testing.Stub

	modelTag names.ModelTag
	cloud    string
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	return m.modelTag
}

func (m *mockModel) CloudName() string {
	m.MethodCall(m, "Cloud")
	return m.cloud
}

func (m *mockModel) WatchModelCredential() state.NotifyWatcher {
	m.MethodCall(m, "WatchModelCredential")
	return apiservertesting.NewFakeNotifyWatcher()
}
