// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"bytes"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/testing"
)

type sshContainerSuite struct {
	testing.BaseSuite

	modelUUID          string
	cloudCredentialAPI *mocks.MockCloudCredentialAPI
	modelAPI           *mocks.MockModelAPI
	applicationAPI     *mocks.MockApplicationAPI
	execClient         *mocks.MockExecutor

	sshC commands.SSHContainerInterfaceForTest
}

var _ = gc.Suite(&sshContainerSuite{})

func (s *sshContainerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.modelUUID = "e0453597-8109-4f7d-a58f-af08bc72a414"
}

func (s *sshContainerSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.cloudCredentialAPI = nil
	s.modelAPI = nil
	s.applicationAPI = nil
	s.execClient = nil
}

func (s *sshContainerSuite) setUpController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.cloudCredentialAPI = mocks.NewMockCloudCredentialAPI(ctrl)
	s.modelAPI = mocks.NewMockModelAPI(ctrl)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	s.execClient = mocks.NewMockExecutor(ctrl)

	s.sshC = commands.NewSSHContainer(
		s.modelUUID,
		s.cloudCredentialAPI,
		s.modelAPI,
		s.applicationAPI,
		func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
			return s.execClient, nil
		},
	)
	return ctrl
}

func (s *sshContainerSuite) TestCleanupRun(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().Close(),
		s.modelAPI.EXPECT().Close(),
		s.applicationAPI.EXPECT().Close(),
	)
	s.sshC.CleanupRun()
}

func (s *sshContainerSuite) TestResolveTarget(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0"},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), gc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestGetExecClient(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	cloudCredentailTag, err := names.ParseCloudCredentialTag("cloudcred-microk8s_admin_microk8s")
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(2),
		s.modelAPI.EXPECT().ModelInfo([]names.ModelTag{names.NewModelTag(s.modelUUID)}).
			Return([]params.ModelInfoResult{
				{Result: &params.ModelInfo{CloudCredentialTag: "cloudcred-microk8s_admin_microk8s"}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().CredentialContents(cloudCredentailTag.Cloud().Id(), cloudCredentailTag.Name(), true).
			Return([]params.CredentialContentResult{
				{Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:     "microk8s",
						AuthType: "certificate",
						Cloud:    "microk8s",
					},
				}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().Cloud(names.NewCloudTag("microk8s")).
			Return(jujucloud.Cloud{
				Name:      "microk8s",
				Type:      "kubernetes",
				AuthTypes: jujucloud.AuthTypes{"certificate"},
			}, nil),
	)
	execC, err := s.sshC.GetExecClient()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(execC, gc.DeepEquals, s.execClient)
}

func (s *sshContainerSuite) TestGetExecClientNotSupportedAPIVersion(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(1),
	)
	_, err := s.sshC.GetExecClient()
	c.Assert(err, gc.ErrorMatches, `credential content lookup on the controller in Juju v1 not supported`)
}

func (s *sshContainerSuite) TestGetExecClientFailedInvalidCredential(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	cloudCredentailTag, err := names.ParseCloudCredentialTag("cloudcred-microk8s_admin_microk8s")
	c.Assert(err, jc.ErrorIsNil)

	notValid := false
	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(2),
		s.modelAPI.EXPECT().ModelInfo([]names.ModelTag{names.NewModelTag(s.modelUUID)}).
			Return([]params.ModelInfoResult{
				{Result: &params.ModelInfo{CloudCredentialTag: "cloudcred-microk8s_admin_microk8s"}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().CredentialContents(cloudCredentailTag.Cloud().Id(), cloudCredentailTag.Name(), true).
			Return([]params.CredentialContentResult{
				{Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:     "microk8s",
						AuthType: "certificate",
						Cloud:    "microk8s",
						Valid:    &notValid,
					},
				}},
			}, nil),
	)
	_, err = s.sshC.GetExecClient()
	c.Assert(err, gc.ErrorMatches, `model credential "microk8s" is not valid`)
}

func (s *sshContainerSuite) TestGetExecClientFailedForNonCAASCloud(c *gc.C) {
	ctrl := s.setUpController(c)
	defer ctrl.Finish()

	cloudCredentailTag, err := names.ParseCloudCredentialTag("cloudcred-microk8s_admin_microk8s")
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(2),
		s.modelAPI.EXPECT().ModelInfo([]names.ModelTag{names.NewModelTag(s.modelUUID)}).
			Return([]params.ModelInfoResult{
				{Result: &params.ModelInfo{CloudCredentialTag: "cloudcred-microk8s_admin_microk8s"}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().CredentialContents(cloudCredentailTag.Cloud().Id(), cloudCredentailTag.Name(), true).
			Return([]params.CredentialContentResult{
				{Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:     "lxd",
						AuthType: "certificate",
						Cloud:    "lxd",
					},
				}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().Cloud(names.NewCloudTag("lxd")).
			Return(jujucloud.Cloud{
				Name:      "lxd",
				Type:      "lxd",
				AuthTypes: jujucloud.AuthTypes{"certificate"},
			}, nil),
	)
	_, err = s.sshC.GetExecClient()
	c.Assert(err, gc.ErrorMatches, `cloud "lxd" is not kubernetes cloud type`)
}

func (s *sshContainerSuite) TestSSH(c *gc.C) {
	ctrl := s.setUpController(c)
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	cloudCredentailTag, err := names.ParseCloudCredentialTag("cloudcred-microk8s_admin_microk8s")
	c.Assert(err, jc.ErrorIsNil)

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(2),
		s.modelAPI.EXPECT().ModelInfo([]names.ModelTag{names.NewModelTag(s.modelUUID)}).
			Return([]params.ModelInfoResult{
				{Result: &params.ModelInfo{CloudCredentialTag: "cloudcred-microk8s_admin_microk8s"}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().CredentialContents(cloudCredentailTag.Cloud().Id(), cloudCredentailTag.Name(), true).
			Return([]params.CredentialContentResult{
				{Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:     "microk8s",
						AuthType: "certificate",
						Cloud:    "microk8s",
					},
				}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().Cloud(names.NewCloudTag("microk8s")).
			Return(jujucloud.Cloud{
				Name:      "microk8s",
				Type:      "kubernetes",
				AuthTypes: jujucloud.AuthTypes{"certificate"},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(k8sexec.ExecParams{
			PodName:  "mariadb-k8s-0",
			Commands: []string{"bash"},
			Tty:      true,
			Stdout:   buffer,
			Stderr:   buffer,
			Stdin:    buffer,
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &commands.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err = s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHCancelled(c *gc.C) {
	ctrl := s.setUpController(c)
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	cloudCredentailTag, err := names.ParseCloudCredentialTag("cloudcred-microk8s_admin_microk8s")
	c.Assert(err, jc.ErrorIsNil)

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(2),
		s.modelAPI.EXPECT().ModelInfo([]names.ModelTag{names.NewModelTag(s.modelUUID)}).
			Return([]params.ModelInfoResult{
				{Result: &params.ModelInfo{CloudCredentialTag: "cloudcred-microk8s_admin_microk8s"}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().CredentialContents(cloudCredentailTag.Cloud().Id(), cloudCredentailTag.Name(), true).
			Return([]params.CredentialContentResult{
				{Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:     "microk8s",
						AuthType: "certificate",
						Cloud:    "microk8s",
					},
				}},
			}, nil),
		s.cloudCredentialAPI.EXPECT().Cloud(names.NewCloudTag("microk8s")).
			Return(jujucloud.Cloud{
				Name:      "microk8s",
				Type:      "kubernetes",
				AuthTypes: jujucloud.AuthTypes{"certificate"},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()).DoAndReturn(
			func(ch chan<- os.Signal) {
				ch <- os.Interrupt
			},
		),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(k8sexec.ExecParams{
			PodName:  "mariadb-k8s-0",
			Commands: []string{"bash"},
			Tty:      true,
			Stdout:   buffer,
			Stderr:   buffer,
			Stdin:    buffer,
		}, gomock.Any()).DoAndReturn(
			func(arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				select {
				case <-cancel:
					return errors.New("cancelled")
				case <-time.After(testing.LongWait):
					c.Fatalf("timed out waiting for Exec return")
				}
				return nil
			},
		),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &commands.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err = s.sshC.SSH(ctx, true, target)
	c.Assert(err, gc.ErrorMatches, `cancelled`)
}
