// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package initialize_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/caasapplication"
	"github.com/juju/juju/cmd/containeragent/initialize"
	"github.com/juju/juju/cmd/containeragent/initialize/mocks"
	utilsmocks "github.com/juju/juju/cmd/containeragent/utils/mocks"
	coretesting "github.com/juju/juju/testing"
)

type initCommandSuit struct {
	coretesting.BaseSuite

	applicationAPI   *mocks.MockApplicationAPI
	fileReaderWriter *utilsmocks.MockFileReaderWriter
	environment      *utilsmocks.MockEnvironment
	cmd              cmd.Command
}

var _ = gc.Suite(&initCommandSuit{})

var podEnv = map[string]string{
	"JUJU_K8S_APPLICATION":          "gitlab",
	"JUJU_K8S_CONTROLLER_CA_CERT":   "ca-cert",
	"JUJU_K8S_CONTROLLER_ADDRESSES": "1.1.1.1,2.2.2.2",
	"JUJU_K8S_APPLICATION_PASSWORD": "passwd",
	"JUJU_K8S_MODEL":                "model1",

	"JUJU_K8S_POD_NAME": "gitlab-0",
	"JUJU_K8S_POD_UUID": "gitlab-uuid",
}

func (s *initCommandSuit) SetUpTest(c *gc.C) {
	for k, v := range podEnv {
		c.Assert(os.Setenv(k, v), jc.ErrorIsNil)
	}
}

func (s *initCommandSuit) TearDownTest(c *gc.C) {
	for k := range podEnv {
		c.Assert(os.Unsetenv(k), jc.ErrorIsNil)
	}

	s.applicationAPI = nil
	s.cmd = nil
}

func (s *initCommandSuit) setupCommand(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	s.fileReaderWriter = utilsmocks.NewMockFileReaderWriter(ctrl)
	s.environment = utilsmocks.NewMockEnvironment(ctrl)
	s.cmd = initialize.NewInitCommandForTest(s.applicationAPI, s.fileReaderWriter, s.environment)
	return ctrl
}

func (s *initCommandSuit) TestRun(c *gc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	data := []byte(`
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: unit-gitlab-0
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
upgradedToVersion: 2.9-beta1
apiaddresses:
- localhost:17070
apiport: 17070`[1:])
	expectedPebble := []byte(`PEBBLE`)
	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Reader("/opt/pebble").Times(1).Return(ioutil.NopCloser(bytes.NewReader(expectedPebble)), nil)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0755)).Return(NopWriteCloser(pebbleWritten), nil)
	s.fileReaderWriter.EXPECT().Reader("/opt/containeragent").Times(1).Return(ioutil.NopCloser(bytes.NewReader(expectedContainerAgent)), nil)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0755)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.fileReaderWriter.EXPECT().Reader("/opt/jujuc").Times(1).Return(ioutil.NopCloser(bytes.NewReader(expectedJujuc)), nil)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0755)).Return(NopWriteCloser(jujucWritten), nil)

	gomock.InOrder(
		s.applicationAPI.EXPECT().UnitIntroduction(`gitlab-0`, `gitlab-uuid`).Times(1).Return(&caasapplication.UnitConfig{
			UnitTag:   names.NewUnitTag("gitlab/0"),
			AgentConf: data,
		}, nil),

		s.fileReaderWriter.EXPECT().MkdirAll("/var/lib/juju", os.FileMode(0755)).Return(nil),
		s.fileReaderWriter.EXPECT().WriteFile("/var/lib/juju/template-agent.conf", data, os.FileMode(0644)).Return(nil),
		s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0755)).Return(nil),

		s.applicationAPI.EXPECT().Close().Times(1).Return(nil),
	)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(pebbleWritten.Bytes(), jc.SameContents, expectedPebble)
	c.Assert(containerAgentWritten.Bytes(), jc.SameContents, expectedContainerAgent)
	c.Assert(jujucWritten.Bytes(), jc.SameContents, expectedJujuc)
}

type nopWriterCloser struct {
	io.Writer
}

var _ io.WriteCloser = (*nopWriterCloser)(nil)

func (*nopWriterCloser) Close() error {
	return nil
}

func NopWriteCloser(w io.Writer) io.WriteCloser {
	return &nopWriterCloser{w}
}
