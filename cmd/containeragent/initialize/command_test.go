// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/agent/caasapplication"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/containeragent/initialize"
	"github.com/juju/juju/cmd/containeragent/initialize/mocks"
	utilsmocks "github.com/juju/juju/cmd/containeragent/utils/mocks"
	coretesting "github.com/juju/juju/internal/testing"
)

type initCommandSuit struct {
	coretesting.BaseSuite

	applicationAPI   *mocks.MockApplicationAPI
	fileReaderWriter *utilsmocks.MockFileReaderWriter
	environment      *utilsmocks.MockEnvironment
	cmd              cmd.Command
	clock            testclock.AdvanceableClock
}

func TestInitCommandSuit(t *testing.T) {
	tc.Run(t, &initCommandSuit{})
}

var podEnv = map[string]string{
	"JUJU_K8S_APPLICATION":          "gitlab",
	"JUJU_K8S_CONTROLLER_CA_CERT":   "ca-cert",
	"JUJU_K8S_CONTROLLER_ADDRESSES": "1.1.1.1,2.2.2.2",
	"JUJU_K8S_APPLICATION_PASSWORD": "passwd",
	"JUJU_K8S_MODEL":                "model1",

	"JUJU_K8S_POD_NAME": "gitlab-0",
	"JUJU_K8S_POD_UUID": "gitlab-uuid",
}

func (s *initCommandSuit) SetUpTest(c *tc.C) {
	for k, v := range podEnv {
		c.Assert(os.Setenv(k, v), tc.ErrorIsNil)
	}
}

func (s *initCommandSuit) TearDownTest(c *tc.C) {
	for k := range podEnv {
		c.Assert(os.Unsetenv(k), tc.ErrorIsNil)
	}

	s.applicationAPI = nil
	s.cmd = nil
}

func (s *initCommandSuit) setupCommand(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	s.fileReaderWriter = utilsmocks.NewMockFileReaderWriter(ctrl)
	s.environment = utilsmocks.NewMockEnvironment(ctrl)
	s.clock = testclock.NewDilatedWallClock(coretesting.ShortWait)
	s.cmd = initialize.NewInitCommandForTest(s.applicationAPI, s.fileReaderWriter, s.environment, s.clock)
	return ctrl
}

// expectFHSBinaryCopy expects the FHS binary shipped by the chiselled rock
// to be read and copied into the charm bin directory.
func (s *initCommandSuit) expectFHSBinaryCopy(src string, data []byte) {
	s.fileReaderWriter.EXPECT().Reader(src).Times(1).
		Return(io.NopCloser(bytes.NewReader(data)), nil)
}

// expectLegacyBinaryCopy expects the FHS lookup to miss and the /opt
// binary of the Docker image to be read and copied into the charm bin
// directory.
func (s *initCommandSuit) expectLegacyBinaryCopy(fhs, legacy string, data []byte) {
	s.fileReaderWriter.EXPECT().Reader(fhs).Times(1).
		Return(nil, os.ErrNotExist)
	s.fileReaderWriter.EXPECT().Reader(legacy).Times(1).
		Return(io.NopCloser(bytes.NewReader(data)), nil)
}

func (s *initCommandSuit) TestRun(c *tc.C) {
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

	expectedCAPebbleService := `summary: Juju container agent service
services:
    container-agent:
        summary: Juju container agent
        startup: enabled
        override: replace
        command: '/charm/bin/containeragent unit --data-dir /var/lib/juju --append-env "PATH=$PATH:/charm/bin" --show-log '
        environment:
            HTTP_PROBE_PORT: "65301"
            PEBBLE_SOCKET: /var/lib/pebble/default/.pebble.socket
        kill-delay: 30m0s
        on-success: ignore
        on-failure: shutdown
        on-check-failure:
            liveness: ignore
            readiness: ignore
checks:
    liveness:
        override: replace
        level: alive
        period: 10s
        timeout: 3s
        threshold: 3
        http:
            url: http://localhost:65301/liveness
    readiness:
        override: replace
        level: ready
        period: 10s
        timeout: 3s
        threshold: 3
        http:
            url: http://localhost:65301/readiness
`
	var y any
	err := yaml.Unmarshal([]byte(expectedCAPebbleService), &y)
	c.Assert(err, tc.ErrorIsNil)

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)
	s.expectFHSBinaryCopy("/usr/lib/juju/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	s.applicationAPI.EXPECT().Close().Times(1).Return(nil)

	gomock.InOrder(
		s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, os.ErrNotExist),
		s.applicationAPI.EXPECT().UnitIntroduction(gomock.Any(), `gitlab-0`, `gitlab-uuid`, ``).Times(1).Return(nil, errors.NotFoundf("pod is being created")),
		s.applicationAPI.EXPECT().UnitIntroduction(gomock.Any(), `gitlab-0`, `gitlab-uuid`, ``).Times(1).Return(nil, errors.NotAssignedf("yo we not needed yet")),
		s.applicationAPI.EXPECT().UnitIntroduction(gomock.Any(), `gitlab-0`, `gitlab-uuid`, ``).Times(1).Return(nil, errors.AlreadyExistsf("yo we dead atm")),
		s.applicationAPI.EXPECT().UnitIntroduction(gomock.Any(), `gitlab-0`, `gitlab-uuid`, ``).Times(1).Return(&caasapplication.UnitConfig{
			UnitTag:   names.NewUnitTag("gitlab/0"),
			AgentConf: data,
		}, nil),

		s.fileReaderWriter.EXPECT().MkdirAll("/var/lib/juju", os.FileMode(0775)).Return(nil),
		s.fileReaderWriter.EXPECT().WriteFile("/var/lib/juju/template-agent.conf", data, os.FileMode(0664)).Return(nil),
		s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil),
		s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).
			DoAndReturn(func(_ string, data []byte, _ os.FileMode) error {
				c.Check(string(data), tc.Equals, expectedCAPebbleService)
				return nil
			}),
	)

	_, err = cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(pebbleWritten.Bytes(), tc.SameContents, expectedPebble)
	c.Assert(containerAgentWritten.Bytes(), tc.SameContents, expectedContainerAgent)
	c.Assert(jujucWritten.Bytes(), tc.SameContents, expectedJujuc)
}

func (s *initCommandSuit) TestRunController(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedPebble := []byte(`PEBBLE`)
	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	expectedCAPebbleService := `summary: Juju container agent service
services:
    container-agent:
        summary: Juju container agent
        startup: enabled
        override: replace
        command: '/charm/bin/containeragent unit --data-dir /var/lib/juju --append-env "PATH=$PATH:/charm/bin" --show-log '
        environment:
            HTTP_PROBE_PORT: "65301"
            PEBBLE_SOCKET: /var/lib/pebble/default/.pebble.socket
        kill-delay: 30m0s
        on-success: ignore
        on-failure: restart
        on-check-failure:
            liveness: ignore
            readiness: ignore
checks:
    liveness:
        override: replace
        level: alive
        period: 10s
        timeout: 3s
        threshold: 3
        http:
            url: http://localhost:65301/liveness
    readiness:
        override: replace
        level: ready
        period: 10s
        timeout: 3s
        threshold: 3
        http:
            url: http://localhost:65301/readiness
`

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	fsInfo := struct{ os.FileInfo }{nil}
	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(&fsInfo, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Return(nil)
	s.expectLegacyBinaryCopy("/usr/lib/juju/pebble", "/opt/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/containeragent", "/opt/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/jujuc", "/opt/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	gomock.InOrder(
		s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil),
		s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).
			DoAndReturn(func(_ string, data []byte, _ os.FileMode) error {
				c.Check(string(data), tc.Equals, expectedCAPebbleService)
				return nil
			}),
	)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
		"--controller",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(pebbleWritten.Bytes(), tc.SameContents, expectedPebble)
	c.Assert(containerAgentWritten.Bytes(), tc.SameContents, expectedContainerAgent)
	c.Assert(jujucWritten.Bytes(), tc.SameContents, expectedJujuc)
}

func (s *initCommandSuit) TestRunConfExists(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedPebble := []byte(`PEBBLE`)
	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)
	s.expectLegacyBinaryCopy("/usr/lib/juju/pebble", "/opt/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/containeragent", "/opt/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/jujuc", "/opt/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)
	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(pebbleWritten.Bytes(), tc.SameContents, expectedPebble)
	c.Assert(containerAgentWritten.Bytes(), tc.SameContents, expectedContainerAgent)
	c.Assert(jujucWritten.Bytes(), tc.SameContents, expectedJujuc)
}

func (s *initCommandSuit) TestInstallProfileFunctions(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedPebble := []byte(`PEBBLE`)
	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)
	s.expectLegacyBinaryCopy("/usr/lib/juju/pebble", "/opt/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/containeragent", "/opt/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/jujuc", "/opt/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)
	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)

	s.fileReaderWriter.EXPECT().ReadFile("/etc/profile.d/juju-introspection.sh").Times(1).Return(nil, os.ErrNotExist)
	s.fileReaderWriter.EXPECT().WriteFile("/etc/profile.d/juju-introspection.sh", gomock.Any(), os.FileMode(0644)).Times(1).Return(nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
		"--profile-dir", "/etc/profile.d/",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(pebbleWritten.Bytes(), tc.SameContents, expectedPebble)
	c.Assert(containerAgentWritten.Bytes(), tc.SameContents, expectedContainerAgent)
	c.Assert(jujucWritten.Bytes(), tc.SameContents, expectedJujuc)
}

func (s *initCommandSuit) TestRunCopyBinariesFailure(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)

	// Both sources of the pebble binary are unreadable: the run must fail
	// loudly, report every attempted source in one message and never open
	// the destination for the failed binary. No Writer expectation is
	// registered for /charm/bin/pebble, so an attempt to write it fails
	// the test.
	s.fileReaderWriter.EXPECT().Reader("/usr/lib/juju/pebble").Times(1).
		Return(nil, os.ErrNotExist)
	s.fileReaderWriter.EXPECT().Reader("/opt/pebble").Times(1).
		Return(nil, os.ErrPermission)

	// The other binaries are still copied: the failing copy must not
	// block the copies that can succeed.
	s.expectFHSBinaryCopy("/usr/bin/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorMatches,
		`copying to "/charm/bin/pebble": opening "/usr/lib/juju/pebble" for reading: file does not exist\nopening "/opt/pebble" for reading: permission denied`)

	c.Check(containerAgentWritten.Bytes(), tc.DeepEquals, expectedContainerAgent)
	c.Check(jujucWritten.Bytes(), tc.DeepEquals, expectedJujuc)
}

func (s *initCommandSuit) TestRunCopyBinariesWriteFailure(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedPebble := []byte(`PEBBLE`)
	expectedJujuc := []byte(`JUJUC`)

	pebbleWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)

	// The FHS source opens but the destination write fails: the run must
	// fail with the copy error and must not fall back to the /opt source
	// of the Docker image. No Reader expectation is registered for
	// /opt/containeragent, so an attempt to read it fails the test.
	s.expectFHSBinaryCopy("/usr/lib/juju/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.fileReaderWriter.EXPECT().Reader("/usr/bin/containeragent").Times(1).
		Return(io.NopCloser(bytes.NewReader([]byte(`CONTAINERAGENT`))), nil)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Times(1).
		Return(nil, os.ErrPermission)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorMatches,
		`copying "/usr/bin/containeragent" to "/charm/bin/containeragent": opening "/charm/bin/containeragent" for writing: permission denied`)

	// The failing copy must not block the copies that succeed.
	c.Check(pebbleWritten.Bytes(), tc.DeepEquals, expectedPebble)
	c.Check(jujucWritten.Bytes(), tc.DeepEquals, expectedJujuc)
}

func (s *initCommandSuit) TestRunMixedBinarySources(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedPebble := []byte(`PEBBLE`)
	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	pebbleWritten := bytes.NewBuffer(nil)
	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)

	// The fallback is resolved per binary: pebble and jujuc come from the
	// FHS layout of the chiselled rock, while containeragent is only
	// available at the /opt path of the Docker image.
	s.expectFHSBinaryCopy("/usr/lib/juju/pebble", expectedPebble)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Return(NopWriteCloser(pebbleWritten), nil)
	s.expectLegacyBinaryCopy("/usr/bin/containeragent", "/opt/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(pebbleWritten.Bytes(), tc.DeepEquals, expectedPebble)
	c.Check(containerAgentWritten.Bytes(), tc.DeepEquals, expectedContainerAgent)
	c.Check(jujucWritten.Bytes(), tc.DeepEquals, expectedJujuc)
}

func (s *initCommandSuit) TestRunCopyBinariesWriterEOF(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)

	// The pebble destination signals EOF mid-copy: the run must fail
	// instead of blessing a truncated binary, and must not fall back to
	// the /opt source of the Docker image. No Reader expectation is
	// registered for /opt/pebble, so an attempt to read it fails the
	// test.
	s.fileReaderWriter.EXPECT().Reader("/usr/lib/juju/pebble").Times(1).
		Return(io.NopCloser(bytes.NewReader([]byte(`PEBBLE`))), nil)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Times(1).
		Return(eofWriter{}, nil)

	// The other binaries are still copied: the failing copy must not
	// block the copies that can succeed.
	s.expectFHSBinaryCopy("/usr/bin/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorMatches, `copying "/usr/lib/juju/pebble" to "/charm/bin/pebble": EOF`)

	c.Check(containerAgentWritten.Bytes(), tc.DeepEquals, expectedContainerAgent)
	c.Check(jujucWritten.Bytes(), tc.DeepEquals, expectedJujuc)
}

func (s *initCommandSuit) TestRunCopyBinariesCloseFailure(c *tc.C) {
	ctrl := s.setupCommand(c)
	defer ctrl.Finish()

	expectedContainerAgent := []byte(`CONTAINERAGENT`)
	expectedJujuc := []byte(`JUJUC`)

	containerAgentWritten := bytes.NewBuffer(nil)
	jujucWritten := bytes.NewBuffer(nil)

	s.fileReaderWriter.EXPECT().Stat("/var/lib/juju/template-agent.conf").Return(nil, nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/containeragent/pebble/layers", os.FileMode(0775)).Return(nil)
	s.fileReaderWriter.EXPECT().WriteFile("/containeragent/pebble/layers/001-container-agent.yaml", gomock.Any(), os.FileMode(0664)).Return(nil)
	s.fileReaderWriter.EXPECT().MkdirAll("/charm/bin", os.FileMode(0775)).Times(1).Return(nil)

	// The pebble copy itself succeeds but the destination Close fails:
	// the flush error must surface instead of being swallowed.
	s.fileReaderWriter.EXPECT().Reader("/usr/lib/juju/pebble").Times(1).
		Return(io.NopCloser(bytes.NewReader([]byte(`PEBBLE`))), nil)
	pebbleWriter := &closeFailWriter{}
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/pebble", os.FileMode(0775)).Times(1).
		Return(pebbleWriter, nil)

	// The other binaries are still copied: the failing copy must not
	// block the copies that can succeed.
	s.expectFHSBinaryCopy("/usr/bin/containeragent", expectedContainerAgent)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/containeragent", os.FileMode(0775)).Return(NopWriteCloser(containerAgentWritten), nil)
	s.expectFHSBinaryCopy("/usr/bin/jujuc", expectedJujuc)
	s.fileReaderWriter.EXPECT().Writer("/charm/bin/jujuc", os.FileMode(0775)).Return(NopWriteCloser(jujucWritten), nil)

	_, err := cmdtesting.RunCommand(c, s.cmd,
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--data-dir", "/var/lib/juju",
		"--bin-dir", "/charm/bin",
	)
	c.Assert(err, tc.ErrorMatches,
		`copying "/usr/lib/juju/pebble" to "/charm/bin/pebble": closing "/charm/bin/pebble": close failed`)

	// The data reached the destination buffer before the failed close.
	c.Check(pebbleWriter.buf.Bytes(), tc.DeepEquals, []byte(`PEBBLE`))
	c.Check(containerAgentWritten.Bytes(), tc.DeepEquals, expectedContainerAgent)
	c.Check(jujucWritten.Bytes(), tc.DeepEquals, expectedJujuc)
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

var errCloseFailed = errors.New("close failed")

// eofWriter fails every write with io.EOF, mimicking a destination that
// signals EOF mid-copy.
type eofWriter struct{}

var _ io.WriteCloser = eofWriter{}

func (eofWriter) Write(p []byte) (int, error) {
	return 0, io.EOF
}

func (eofWriter) Close() error {
	return nil
}

// closeFailWriter buffers writes and fails on Close, mimicking a
// destination whose close-time flush fails.
type closeFailWriter struct {
	buf bytes.Buffer
}

var _ io.WriteCloser = (*closeFailWriter)(nil)

func (w *closeFailWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *closeFailWriter) Close() error {
	return errCloseFailed
}
