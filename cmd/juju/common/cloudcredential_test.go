// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
)

func TestCloudCredentialSuite(t *stdtesting.T) { tc.Run(t, &cloudCredentialSuite{}) }

type cloudCredentialSuite struct {
	testhelpers.IsolationSuite
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTag(c *tc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"foo",
		"aws/admin/foo",
	)
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTagOtherUser(c *tc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"brenda/foo",
		"aws/brenda/foo",
	)
}

func testResolveCloudCredentialTag(
	c *tc.C,
	user names.UserTag,
	cloud names.CloudTag,
	credentialName string,
	expect string,
) {
	tag, err := common.ResolveCloudCredentialTag(user, cloud, credentialName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.Id(), tc.Equals, expect)
}

func (*cloudCredentialSuite) TestRegisterCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credential := &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"admin": cloud.NewCredential("certificate", map[string]string{
				"cert": "certificate",
			}),
		},
	}

	mockProvider := common.NewMockTestCloudProvider(ctrl)
	mockProvider.EXPECT().RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(map[string]*cloud.CloudCredential{
		"fake": credential,
	}, nil)
	mockStore := common.NewMockCredentialStore(ctrl)
	mockStore.EXPECT().UpdateCredential("fake", *credential).Return(nil)

	stderr := new(bytes.Buffer)

	err := common.RegisterCredentials(&cmd.Context{
		Context: c.Context(),
		Stderr:  stderr,
	}, mockStore, mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stderr.String(), tc.Equals, "")
}

func (*cloudCredentialSuite) TestRegisterCredentialsWithNoCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockProvider := common.NewMockTestCloudProvider(ctrl)
	mockProvider.EXPECT().RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(map[string]*cloud.CloudCredential{}, nil)
	mockStore := common.NewMockCredentialStore(ctrl)

	stderr := new(bytes.Buffer)

	err := common.RegisterCredentials(&cmd.Context{
		Stderr: stderr,
	}, mockStore, mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (*cloudCredentialSuite) TestRegisterCredentialsWithCallFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockProvider := common.NewMockTestCloudProvider(ctrl)
	mockProvider.EXPECT().RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(nil, errors.New("bad"))
	mockStore := common.NewMockCredentialStore(ctrl)

	stderr := new(bytes.Buffer)

	err := common.RegisterCredentials(&cmd.Context{
		Stderr: stderr,
	}, mockStore, mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(errors.Cause(err).Error(), tc.Matches, "bad")
}

func (*cloudCredentialSuite) assertInvalidCredentialName(c *tc.C, in modelcmd.GetCredentialsParams) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cloudCredential := &cloud.CloudCredential{AuthCredentials: map[string]cloud.Credential{"new one": cloud.NewEmptyCredential()}}
	mockProvider := common.NewMockTestCloudProvider(ctrl)
	mockStore := common.NewMockCredentialStore(ctrl)
	mockStore.EXPECT().CredentialForCloud("cloud").Return(
		cloudCredential,
		nil,
	)

	stderr := new(bytes.Buffer)

	_, _, _, _, err := common.GetOrDetectCredential(
		&cmd.Context{Stderr: stderr},
		mockStore,
		mockProvider,
		in,
	)
	c.Assert(errors.Cause(err), tc.ErrorMatches, `credential name "new one" not valid`)
	c.Assert(errors.Cause(err), tc.ErrorIs, errors.NotValid)
}

func (s *cloudCredentialSuite) TestGetOrDetectCredentialInvalidCredentialNameProvided(c *tc.C) {
	s.assertInvalidCredentialName(c,
		modelcmd.GetCredentialsParams{
			CredentialName: "new one",
			Cloud:          cloud.Cloud{Name: "cloud", Type: "dummy"},
		},
	)
}

func (s *cloudCredentialSuite) TestGetOrDetectCredentialInvalidCredentialName(c *tc.C) {
	s.assertInvalidCredentialName(c,
		modelcmd.GetCredentialsParams{
			Cloud: cloud.Cloud{Name: "cloud", Type: "dummy"},
		},
	)
}

func (s *cloudCredentialSuite) TestParseBoolPointer(c *tc.C) {
	_true := true
	_false := false
	for _, t := range []struct {
		pointer  *bool
		trueV    string
		falseV   string
		expected string
	}{
		{nil, "a", "b", "b"},
		{&_false, "a", "b", "b"},
		{&_true, "a", "b", "a"},
	} {

		got := common.HumanReadableBoolPointer(t.pointer, t.trueV, t.falseV)
		c.Assert(got, tc.Equals, t.expected)
	}
}
