// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"bytes"
	"context"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/common/charms"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	k8smocks "github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/cmd/juju/ssh"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type sshContainerSuite struct {
	testing.BaseSuite

	modelUUID      string
	modelName      string
	applicationAPI *mocks.MockApplicationAPI
	charmAPI       *mocks.MockCharmAPI
	execClient     *mocks.MockExecutor
	mockPods       *k8smocks.MockPodInterface
	mockNamespaces *k8smocks.MockNamespaceInterface
	mockSSHClient  *mocks.MockSSHClientAPI
	controllerAPI  *mocks.MockSSHControllerAPI

	sshC ssh.SSHContainerInterfaceForTest
}

var _ = tc.Suite(&sshContainerSuite{})

func (s *sshContainerSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.modelUUID = "e0453597-8109-4f7d-a58f-af08bc72a414"
}

func (s *sshContainerSuite) SetUpTest(c *tc.C) {
	s.modelName = "test"
}

func (s *sshContainerSuite) TearDownTest(c *tc.C) {
	s.BaseSuite.TearDownTest(c)
	s.applicationAPI = nil
	s.execClient = nil
	s.mockPods = nil
	s.mockNamespaces = nil
}

func (s *sshContainerSuite) setUpController(c *tc.C, containerName string) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	s.charmAPI = mocks.NewMockCharmAPI(ctrl)

	s.execClient = mocks.NewMockExecutor(ctrl)

	s.mockPods = k8smocks.NewMockPodInterface(ctrl)
	s.mockNamespaces = k8smocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1 := k8smocks.NewMockCoreV1Interface(ctrl)

	k8sClient := k8smocks.NewMockInterface(ctrl)
	s.execClient.EXPECT().RawClient().AnyTimes().Return(k8sClient)
	k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)
	mockCoreV1.EXPECT().Pods(gomock.Any()).AnyTimes().Return(s.mockPods)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockSSHClient = mocks.NewMockSSHClientAPI(ctrl)
	s.controllerAPI = mocks.NewMockSSHControllerAPI(ctrl)

	s.sshC = ssh.NewSSHContainer(
		s.modelUUID,
		s.modelName,
		s.applicationAPI,
		s.charmAPI,
		s.execClient,
		s.mockSSHClient,
		containerName,
		s.controllerAPI,
	)
	return ctrl
}

func (s *sshContainerSuite) TestCleanupRun(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().Close(),
		s.charmAPI.EXPECT().Close(),
		s.mockSSHClient.EXPECT().Close(),
	)
	s.sshC.CleanupRun()
}

func (s *sshContainerSuite) TestResolveTargetForWorkloadPod(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), tc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForController(c *tc.C) {
	s.modelName = "controller"
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	target, err := s.sshC.ResolveTarget(context.Background(), "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), tc.DeepEquals, "controller-0")
}

func (s *sshContainerSuite) TestResolveTargetForControllerInvalidTarget(c *tc.C) {
	s.modelName = "controller"
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	_, err := s.sshC.ResolveTarget(context.Background(), "1")
	c.Assert(err, tc.ErrorMatches, `target "1" not found`)
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharm(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Manifest: &charm.Manifest{
					Bases: []charm.Base{{
						Name: "ubuntu",
						Channel: charm.Channel{
							Track: "20.04",
							Risk:  "stable",
						},
					}},
				},
				Meta: &charm.Meta{},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), tc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveCharmTargetForSidecarCharm(c *tc.C) {
	ctrl := s.setUpController(c, "charm")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Manifest: &charm.Manifest{
					Bases: []charm.Base{{
						Name: "ubuntu",
						Channel: charm.Channel{
							Track: "20.04",
							Risk:  "stable",
						},
					}},
				},
				Meta: &charm.Meta{},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), tc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharmWithContainer(c *tc.C) {
	ctrl := s.setUpController(c, "test-container")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Containers: map[string]charm.Container{
						"test-container": {},
					},
				},
				Manifest: &charm.Manifest{
					Bases: []charm.Base{{
						Name: "ubuntu",
						Channel: charm.Channel{
							Track: "20.04",
							Risk:  "stable",
						},
					}},
				},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), tc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharmWithContainerMissing(c *tc.C) {
	ctrl := s.setUpController(c, "bad-test-container")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Containers: map[string]charm.Container{
						"test-container": {},
					},
				},
				Manifest: &charm.Manifest{
					Bases: []charm.Base{{
						Name: "ubuntu",
						Channel: charm.Channel{
							Track: "20.04",
							Risk:  "stable",
						},
					}},
				},
			}, nil),
	)
	_, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, tc.ErrorMatches, `container "bad-test-container" must be one of charm, test-container`)
}

func (s *sshContainerSuite) TestResolveTargetForWorkloadPodNoProviderID(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
	)
	_, err := s.sshC.ResolveTarget(context.Background(), "mariadb-k8s/0")
	c.Assert(err, tc.ErrorMatches, `container for unit "mariadb-k8s/0" is not ready yet`)
}

func (s *sshContainerSuite) TestGetExecClient(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockSSHClient.EXPECT().ModelCredentialForSSH(gomock.Any()).
			Return(cloudspec.CloudSpec{}, nil),
	)
	execC, err := s.sshC.GetExecClient()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.sshC.ModelName(), tc.Equals, s.modelName)
	c.Assert(execC, tc.DeepEquals, s.execClient)
}

func (s *sshContainerSuite) TestSSHNoContainerSpecified(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				mc := jc.NewMultiChecker()
				mc.AddExpr(`_.Env`, jc.Ignore)
				c.Check(arg, mc, k8sexec.ExecParams{
					PodName:  "mariadb-k8s-0",
					Commands: []string{"bash"},
					TTY:      true,
					Stdout:   buffer,
					Stderr:   buffer,
					Stdin:    buffer,
				})
				return nil
			}),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &ssh.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHWithContainerSpecified(c *tc.C) {
	ctrl := s.setUpController(c, "container1")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				mc := jc.NewMultiChecker()
				mc.AddExpr(`_.Env`, jc.Ignore)
				c.Check(arg, mc, k8sexec.ExecParams{
					PodName:       "mariadb-k8s-0",
					ContainerName: "container1",
					Commands:      []string{"bash"},
					TTY:           true,
					Stdout:        buffer,
					Stderr:        buffer,
					Stdin:         buffer,
				})
				return nil
			}),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &ssh.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHCancelled(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()).DoAndReturn(
			func(ch chan<- os.Signal) {
				ch <- os.Interrupt
			},
		),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				mc := jc.NewMultiChecker()
				mc.AddExpr(`_.Env`, jc.Ignore)
				c.Check(arg, mc, k8sexec.ExecParams{
					PodName:  "mariadb-k8s-0",
					Commands: []string{"bash"},
					TTY:      true,
					Stdout:   buffer,
					Stderr:   buffer,
					Stdin:    buffer,
				})
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

	target := &ssh.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, tc.ErrorMatches, `cancelled`)
}

func (s *sshContainerSuite) TestGetInterruptAbortChanInterrupted(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()).DoAndReturn(
			func(ch chan<- os.Signal) {
				ch <- os.Interrupt
			},
		),
	)
	cancel, _ := ssh.GetInterruptAbortChan(ctx)

	select {
	case _, ok := <-cancel:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for cancelling")
	}
}

func (s *sshContainerSuite) TestGetInterruptAbortChanStopped(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)
	cancel, stop := ssh.GetInterruptAbortChan(ctx)
	stop()
	select {
	case _, ok := <-cancel:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for cancelling")
	}
}

func (s *sshContainerSuite) TestCopyInvalidArgs(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1"})
	err := s.sshC.Copy(ctx)
	c.Assert(err, tc.ErrorMatches, `source and destination are required`)

	s.sshC.SetArgs([]string{"./file1", "./file2", "mariadb-k8s/0:/home/ubuntu/"})
	err = s.sshC.Copy(ctx)
	c.Assert(err, tc.ErrorMatches, `only one source and one destination are allowed for a k8s application`)
}

func (s *sshContainerSuite) TestCopyFromWorkloadPod(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"mariadb-k8s/0:/home/ubuntu/", "./file1"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(gomock.Any(), k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0", ContainerName: "charm"},
			Dest: k8sexec.FileResource{Path: "./file1"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyToWorkloadPod(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1", "mariadb-k8s/0:/home/ubuntu/"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(gomock.Any(), k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "./file1"},
			Dest: k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0", ContainerName: "charm"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyToWorkloadPodWithContainerSpecified(c *tc.C) {
	ctrl := s.setUpController(c, "container1")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1", "mariadb-k8s/0:/home/ubuntu/"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo(gomock.Any(), []names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo(gomock.Any(), "test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Containers: map[string]charm.Container{
						"container1": {},
					},
				},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(gomock.Any(), k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "./file1"},
			Dest: k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0", ContainerName: "container1"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestNamespaceControllerModel(c *tc.C) {
	ctrl := s.setUpController(c, "")
	defer ctrl.Finish()

	mc := mocks.NewMockModelCommand(ctrl)
	mc.EXPECT().ModelIdentifier().Return("admin/controller", nil)
	mc.EXPECT().ControllerDetails().Return(&jujuclient.ControllerDetails{
		ControllerUUID: "badf00d",
	}, nil)
	mc.EXPECT().NewControllerAPIRoot(gomock.Any()).Return(nil, nil)
	mc.EXPECT().NewAPIRoot(gomock.Any()).Return(nil, nil)
	s.controllerAPI.EXPECT().ControllerConfig(gomock.Any()).Return(
		controller.Config{"controller-name": "foobar"}, nil)

	err := s.sshC.InitRun(context.Background(), mc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.sshC.Namespace(), tc.Equals, "controller-foobar")
}

func (s *sshContainerSuite) TestSSHWithTerm(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	prevTerm, prevTermSet := os.LookupEnv("TERM")
	os.Setenv("TERM", "foobar-256color")
	s.AddCleanup(func(c *tc.C) {
		if prevTermSet {
			os.Setenv("TERM", prevTerm)
		} else {
			os.Unsetenv("TERM")
		}
	})

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				c.Check(arg, jc.DeepEquals, k8sexec.ExecParams{
					PodName:  "mariadb-k8s-0",
					Env:      []string{"TERM=foobar-256color"},
					Commands: []string{"bash"},
					TTY:      true,
					Stdout:   buffer,
					Stderr:   buffer,
					Stdin:    buffer,
				})
				return nil
			}),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &ssh.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHWithTermNoTTY(c *tc.C) {
	ctrl := s.setUpController(c, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	prevTerm, prevTermSet := os.LookupEnv("TERM")
	os.Setenv("TERM", "foobar-256color")
	s.AddCleanup(func(c *tc.C) {
		if prevTermSet {
			os.Setenv("TERM", prevTerm)
		} else {
			os.Unsetenv("TERM")
		}
	})

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, arg k8sexec.ExecParams, cancel <-chan struct{}) error {
				c.Check(arg, jc.DeepEquals, k8sexec.ExecParams{
					PodName:  "mariadb-k8s-0",
					Env:      nil,
					Commands: []string{"bash"},
					TTY:      false,
					Stdout:   buffer,
					Stderr:   buffer,
					Stdin:    buffer,
				})
				return nil
			}),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &ssh.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, false, target)
	c.Assert(err, jc.ErrorIsNil)
}
