// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	_ "github.com/juju/juju/provider/dummy"
)

var _ = gc.Suite(&cloudCredentialSuite{})

type cloudCredentialSuite struct {
	testing.IsolationSuite
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTag(c *gc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"foo",
		"aws/admin/foo",
	)
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTagOtherUser(c *gc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"brenda/foo",
		"aws/brenda/foo",
	)
}

func testResolveCloudCredentialTag(
	c *gc.C,
	user names.UserTag,
	cloud names.CloudTag,
	credentialName string,
	expect string,
) {
	tag, err := common.ResolveCloudCredentialTag(user, cloud, credentialName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.Id(), gc.Equals, expect)
}

func (*cloudCredentialSuite) TestRegisterCredentials(c *gc.C) {
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
		Stderr: stderr,
	}, mockStore, mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stderr.String(), gc.Equals, "")
}

func (*cloudCredentialSuite) TestRegisterCredentialsWithNoCredentials(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
}

func (*cloudCredentialSuite) TestRegisterCredentialsWithCallFailure(c *gc.C) {
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
	c.Assert(errors.Cause(err).Error(), gc.Matches, "bad")
}

func (*cloudCredentialSuite) assertInvalidCredentialName(c *gc.C, in modelcmd.GetCredentialsParams) {
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
	c.Assert(errors.Cause(err), gc.ErrorMatches, `credential name "new one" not valid`)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (s *cloudCredentialSuite) TestGetOrDetectCredentialInvalidCredentialNameProvided(c *gc.C) {
	s.assertInvalidCredentialName(c,
		modelcmd.GetCredentialsParams{
			CredentialName: "new one",
			Cloud:          cloud.Cloud{Name: "cloud", Type: "dummy"},
		},
	)
}

func (s *cloudCredentialSuite) TestGetOrDetectCredentialInvalidCredentialName(c *gc.C) {
	s.assertInvalidCredentialName(c,
		modelcmd.GetCredentialsParams{
			Cloud: cloud.Cloud{Name: "cloud", Type: "dummy"},
		},
	)
}

func (s *cloudCredentialSuite) TestParseBoolPointer(c *gc.C) {
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
		c.Assert(got, gc.Equals, t.expected)
	}
}
