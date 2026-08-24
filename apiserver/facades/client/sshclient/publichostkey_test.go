// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/client/sshclient"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/virtualhostname"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// publicHostKeySuite exercises the PublicHostKeyForTarget facade method used by
// the CLI jump provider to resolve the terminating target host key.
type publicHostKeySuite struct {
	modelSSH *MockModelSSHService
}

func TestPublicHostKeySuite(t *stdtesting.T) {
	tc.Run(t, &publicHostKeySuite{})
}

func (s *publicHostKeySuite) newFacade(c *tc.C) (*sshclient.Facade, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.modelSSH = NewMockModelSSHService(ctrl)

	adminTag := names.NewUserTag("admin")
	auth := apiservertesting.FakeAuthorizer{
		Tag:      adminTag,
		AdminTag: adminTag,
	}

	facade, err := sshclient.InternalFacade(
		coretesting.ControllerTag,
		coretesting.ModelTag,
		nil, // ApplicationService, unused by PublicHostKeyForTarget.
		nil, // MachineService, unused.
		nil, // NetworkService, unused.
		nil, // ModelConfigService, unused.
		nil, // ModelProviderService, unused.
		s.modelSSH,
		auth,
	)
	c.Assert(err, tc.ErrorIsNil)
	return facade, ctrl
}

func (s *publicHostKeySuite) TestPublicHostKeyForTarget(c *tc.C) {
	facade, ctrl := s.newFacade(c)
	defer ctrl.Finish()

	info, err := virtualhostname.NewInfoMachineTarget(coretesting.ModelTag.Id(), "0")
	c.Assert(err, tc.ErrorIsNil)

	s.modelSSH.EXPECT().VirtualHostKey(gomock.Any(), info).Return(coretesting.SSHServerHostKey, nil)

	result, err := facade.PublicHostKeyForTarget(c.Context(), params.SSHVirtualHostKeyRequestArg{
		Hostname: info.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)

	wantPublicKey, err := pkissh.MarshalPublicKey([]byte(coretesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.PublicKey, tc.DeepEquals, wantPublicKey)
}

func (s *publicHostKeySuite) TestPublicHostKeyForTargetInvalidHostname(c *tc.C) {
	facade, ctrl := s.newFacade(c)
	defer ctrl.Finish()

	// An unparseable hostname must fail before any service is consulted.
	result, err := facade.PublicHostKeyForTarget(c.Context(), params.SSHVirtualHostKeyRequestArg{
		Hostname: "not-a-valid-hostname",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.PublicKey, tc.IsNil)
}

func (s *publicHostKeySuite) TestPublicHostKeyForTargetVirtualHostKeyError(c *tc.C) {
	facade, ctrl := s.newFacade(c)
	defer ctrl.Finish()

	info, err := virtualhostname.NewInfoMachineTarget(coretesting.ModelTag.Id(), "0")
	c.Assert(err, tc.ErrorIsNil)

	s.modelSSH.EXPECT().VirtualHostKey(gomock.Any(), info).Return("", errors.New("boom"))

	result, err := facade.PublicHostKeyForTarget(c.Context(), params.SSHVirtualHostKeyRequestArg{
		Hostname: info.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.Error.Message, tc.Equals, "boom")
}
