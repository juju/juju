// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"bytes"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/systems"
	"github.com/juju/systems/channel"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/apiserver/params"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	k8smocks "github.com/juju/juju/caas/kubernetes/provider/mocks"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/testing"
)

type sshContainerSuite struct {
	testing.BaseSuite

	modelUUID          string
	cloudCredentialAPI *mocks.MockCloudCredentialAPI
	modelAPI           *mocks.MockModelAPI
	applicationAPI     *mocks.MockApplicationAPI
	charmAPI           *mocks.MockCharmsAPI
	execClient         *mocks.MockExecutor
	mockPods           *k8smocks.MockPodInterface
	mockNamespaces     *k8smocks.MockNamespaceInterface

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
	s.mockPods = nil
	s.mockNamespaces = nil
}

func (s *sshContainerSuite) setUpController(c *gc.C, remote bool, containerName string) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.cloudCredentialAPI = mocks.NewMockCloudCredentialAPI(ctrl)
	s.modelAPI = mocks.NewMockModelAPI(ctrl)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	s.charmAPI = mocks.NewMockCharmsAPI(ctrl)

	s.execClient = mocks.NewMockExecutor(ctrl)

	s.mockPods = k8smocks.NewMockPodInterface(ctrl)
	s.mockNamespaces = k8smocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1 := k8smocks.NewMockCoreV1Interface(ctrl)

	k8sClient := k8smocks.NewMockInterface(ctrl)
	s.execClient.EXPECT().RawClient().AnyTimes().Return(k8sClient)
	k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)
	mockCoreV1.EXPECT().Pods(gomock.Any()).AnyTimes().Return(s.mockPods)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.sshC = commands.NewSSHContainer(
		s.modelUUID,
		s.cloudCredentialAPI,
		s.modelAPI,
		s.applicationAPI,
		s.charmAPI,
		s.execClient,
		remote,
		containerName,
	)
	return ctrl
}

func (s *sshContainerSuite) TestCleanupRun(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().Close(),
		s.modelAPI.EXPECT().Close(),
		s.applicationAPI.EXPECT().Close(),
		s.charmAPI.EXPECT().Close(),
	)
	s.sshC.CleanupRun()
}

func (s *sshContainerSuite) TestResolveTargetForWorkloadPod(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), gc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharm(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Bases: []systems.Base{{
						Name:    "ubuntu",
						Channel: channel.MustParse("20.04/stable"),
					}},
				},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), gc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharmWithContainer(c *gc.C) {
	ctrl := s.setUpController(c, true, "test-container")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Bases: []systems.Base{{
						Name:    "ubuntu",
						Channel: channel.MustParse("20.04/stable"),
					}},
					Containers: map[string]charm.Container{
						"test-container": charm.Container{},
					},
				},
			}, nil),
	)
	target, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), gc.DeepEquals, "mariadb-k8s-0")
}

func (s *sshContainerSuite) TestResolveTargetForSidecarCharmWithContainerMissing(c *gc.C) {
	ctrl := s.setUpController(c, true, "bad-test-container")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{
					Bases: []systems.Base{{
						Name:    "ubuntu",
						Channel: channel.MustParse("20.04/stable"),
					}},
					Containers: map[string]charm.Container{
						"test-container": charm.Container{},
					},
				},
			}, nil),
	)
	_, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, gc.ErrorMatches, `container "bad-test-container" must be one of charm, test-container`)
}

func (s *sshContainerSuite) TestResolveTargetForOperatorPod(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
		s.execClient.EXPECT().NameSpace().AnyTimes().Return("test-ns"),

		s.mockNamespaces.EXPECT().Get(gomock.Any(), gomock.Any(), metav1.GetOptions{}).
			Return(nil, k8serrors.NewNotFound(schema.GroupResource{}, "test")),

		s.mockPods.EXPECT().List(gomock.Any(), metav1.ListOptions{LabelSelector: "operator.juju.is/name=mariadb-k8s,operator.juju.is/target=application"}).AnyTimes().
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "mariadb-k8s-operator-0"}},
			}}, nil),
	)
	target, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.GetEntity(), gc.DeepEquals, "mariadb-k8s-operator-0")
}

func (s *sshContainerSuite) TestResolveTargetForOperatorPodNoProviderID(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
		s.execClient.EXPECT().NameSpace().AnyTimes().Return("test-ns"),

		s.mockNamespaces.EXPECT().Get(gomock.Any(), gomock.Any(), metav1.GetOptions{}).
			Return(nil, k8serrors.NewNotFound(schema.GroupResource{}, "test")),

		s.mockPods.EXPECT().List(gomock.Any(), metav1.ListOptions{LabelSelector: "operator.juju.is/name=mariadb-k8s,operator.juju.is/target=application"}).AnyTimes().
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: ""}},
			}}, nil),
	)
	_, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, gc.ErrorMatches, `operator pod for unit "mariadb-k8s/0" is not ready yet`)
}

func (s *sshContainerSuite) TestResolveTargetForWorkloadPodNoProviderID(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),
	)
	_, err := s.sshC.ResolveTarget("mariadb-k8s/0")
	c.Assert(err, gc.ErrorMatches, `container for unit "mariadb-k8s/0" is not ready yet`)
}

func (s *sshContainerSuite) TestGetExecClient(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
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
	ctrl := s.setUpController(c, true, "")
	defer ctrl.Finish()

	gomock.InOrder(
		s.cloudCredentialAPI.EXPECT().BestAPIVersion().
			Return(1),
	)
	_, err := s.sshC.GetExecClient()
	c.Assert(err, gc.ErrorMatches, `credential content lookup on the controller in Juju v1 not supported`)
}

func (s *sshContainerSuite) TestGetExecClientFailedInvalidCredential(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
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
	ctrl := s.setUpController(c, true, "")
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

func (s *sshContainerSuite) TestSSHNoContainerSpecified(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(k8sexec.ExecParams{
			PodName:  "mariadb-k8s-0",
			Commands: []string{"bash"},
			TTY:      true,
			Stdout:   buffer,
			Stderr:   buffer,
			Stdin:    buffer,
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &commands.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHWithContainerSpecified(c *gc.C) {
	ctrl := s.setUpController(c, true, "container1")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"bash"})

	buffer := bytes.NewBuffer(nil)

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().GetStdout().Return(buffer),
		ctx.EXPECT().GetStderr().Return(buffer),
		ctx.EXPECT().GetStdin().Return(buffer),
		s.execClient.EXPECT().Exec(k8sexec.ExecParams{
			PodName:       "mariadb-k8s-0",
			ContainerName: "container1",
			Commands:      []string{"bash"},
			TTY:           true,
			Stdout:        buffer,
			Stderr:        buffer,
			Stdin:         buffer,
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	target := &commands.ResolvedTarget{}
	target.SetEntity("mariadb-k8s-0")
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestSSHCancelled(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
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
		s.execClient.EXPECT().Exec(k8sexec.ExecParams{
			PodName:  "mariadb-k8s-0",
			Commands: []string{"bash"},
			TTY:      true,
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
	err := s.sshC.SSH(ctx, true, target)
	c.Assert(err, gc.ErrorMatches, `cancelled`)
}

func (s *sshContainerSuite) TestGetInterruptAbortChanInterrupted(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()).DoAndReturn(
			func(ch chan<- os.Signal) {
				ch <- os.Interrupt
			},
		),
	)
	cancel, _ := commands.GetInterruptAbortChan(ctx)

	select {
	case _, ok := <-cancel:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for cancelling")
	}
}

func (s *sshContainerSuite) TestGetInterruptAbortChanStopped(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	gomock.InOrder(
		ctx.EXPECT().InterruptNotify(gomock.Any()),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)
	cancel, stop := commands.GetInterruptAbortChan(ctx)
	stop()
	select {
	case _, ok := <-cancel:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for cancelling")
	}
}

func (s *sshContainerSuite) TestCopyToOperator(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1", "mariadb-k8s/0:/home/ubuntu/"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		s.execClient.EXPECT().NameSpace().AnyTimes().Return("test-ns"),

		s.mockNamespaces.EXPECT().Get(gomock.Any(), gomock.Any(), metav1.GetOptions{}).
			Return(nil, k8serrors.NewNotFound(schema.GroupResource{}, "test")),

		s.mockPods.EXPECT().List(gomock.Any(), metav1.ListOptions{LabelSelector: "operator.juju.is/name=mariadb-k8s,operator.juju.is/target=application"}).AnyTimes().
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "mariadb-k8s-operator-0"}},
			}}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "./file1"},
			Dest: k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-operator-0"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyFromOperator(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"mariadb-k8s/0:/home/ubuntu/", "./file1"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		s.execClient.EXPECT().NameSpace().AnyTimes().Return("test-ns"),

		s.mockNamespaces.EXPECT().Get(gomock.Any(), gomock.Any(), metav1.GetOptions{}).
			Return(nil, k8serrors.NewNotFound(schema.GroupResource{}, "test")),

		s.mockPods.EXPECT().List(gomock.Any(), metav1.ListOptions{LabelSelector: "operator.juju.is/name=mariadb-k8s,operator.juju.is/target=application"}).AnyTimes().
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "mariadb-k8s-operator-0"}},
			}}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-operator-0"},
			Dest: k8sexec.FileResource{Path: "./file1"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyInvalidArgs(c *gc.C) {
	ctrl := s.setUpController(c, false, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1"})
	err := s.sshC.Copy(ctx)
	c.Assert(err, gc.ErrorMatches, `source and destination are required`)

	s.sshC.SetArgs([]string{"./file1", "./file2", "mariadb-k8s/0:/home/ubuntu/"})
	err = s.sshC.Copy(ctx)
	c.Assert(err, gc.ErrorMatches, `only one source and one destination are allowed for a k8s application`)
}

func (s *sshContainerSuite) TestCopyFromWorkloadPod(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"mariadb-k8s/0:/home/ubuntu/", "./file1"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0"},
			Dest: k8sexec.FileResource{Path: "./file1"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyToWorkloadPod(c *gc.C) {
	ctrl := s.setUpController(c, true, "")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1", "mariadb-k8s/0:/home/ubuntu/"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "./file1"},
			Dest: k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *sshContainerSuite) TestCopyToWorkloadPodWithContainerSpecified(c *gc.C) {
	ctrl := s.setUpController(c, true, "container1")
	ctx := mocks.NewMockContext(ctrl)
	defer ctrl.Finish()

	s.sshC.SetArgs([]string{"./file1", "mariadb-k8s/0:/home/ubuntu/"})

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitsInfo([]names.UnitTag{names.NewUnitTag("mariadb-k8s/0")}).
			Return([]application.UnitInfo{
				{ProviderId: "mariadb-k8s-0", Charm: "test-charm-url"},
			}, nil),
		s.charmAPI.EXPECT().CharmInfo("test-charm-url").
			Return(&charms.CharmInfo{
				Meta: &charm.Meta{},
			}, nil),

		ctx.EXPECT().InterruptNotify(gomock.Any()),
		s.execClient.EXPECT().Copy(k8sexec.CopyParams{
			Src:  k8sexec.FileResource{Path: "./file1"},
			Dest: k8sexec.FileResource{Path: "/home/ubuntu/", PodName: "mariadb-k8s-0", ContainerName: "container1"},
		}, gomock.Any()).
			Return(nil),
		ctx.EXPECT().StopInterruptNotify(gomock.Any()),
	)

	err := s.sshC.Copy(ctx)
	c.Assert(err, jc.ErrorIsNil)
}
