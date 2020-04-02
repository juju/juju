// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/juju/clock/testclock"
	"github.com/juju/replicaset"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var _ = gc.Suite(&RestoreSuite{})

type RestoreSuite struct {
	coretesting.BaseSuite
	cwd       string
	testFiles []string
}

func (r *RestoreSuite) SetUpSuite(c *gc.C) {
	r.BaseSuite.SetUpSuite(c)
}

func (r *RestoreSuite) SetUpTest(c *gc.C) {
	r.cwd = c.MkDir()
	r.BaseSuite.SetUpTest(c)
}

func (r *RestoreSuite) TestReplicasetIsReset(c *gc.C) {
	server := &gitjujutesting.MgoInstance{Params: []string{"--replSet", "juju"}}
	err := server.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer server.DestroyWithLog()
	mgoAddr := server.Addr()
	dialInfo := server.DialInfo()

	var cfg *replicaset.Config
	dialInfo = server.DialInfo()
	dialInfo.Addrs = []string{mgoAddr}
	err = resetReplicaSet(dialInfo, mgoAddr)
	c.Assert(err, jc.ErrorIsNil)

	session, err := server.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()
	cfg, err = replicaset.CurrentConfig(session)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Members, gc.HasLen, 1)
	c.Assert(cfg.Members[0].Address, gc.Equals, mgoAddr)
}

type backupConfigTests struct {
	yamlFile      io.Reader
	expectedError error
	message       string
}

func (r *RestoreSuite) TestSetAgentAddressScript(c *gc.C) {
	testServerAddresses := []string{
		"FirstNewControllerAddress:30303",
		"SecondNewControllerAddress:30304",
		"ThirdNewControllerAddress:30305",
		"FourthNewControllerAddress:30306",
		"FiftNewControllerAddress:30307",
		"SixtNewControllerAddress:30308",
	}
	for _, address := range testServerAddresses {
		template := setAgentAddressScript(address, address)
		expectedString := fmt.Sprintf("\t\ts/- .*(:[0-9]+)/- %s\\1/\n", address)
		logger.Infof(fmt.Sprintf("Testing with address %q", address))
		c.Assert(strings.Contains(template, expectedString), gc.Equals, true)
	}
}

func (r *RestoreSuite) TestNewDialInfo(c *gc.C) {

	cases := []struct {
		machineTag       string
		apiPassword      string
		oldPassword      string
		expectedPassword string
		expectedUser     string
		expectedError    string
	}{
		{"machine-0",
			"",
			"123456",
			"123456",
			"admin",
			"",
		},
		{"machine-1",
			"123123",
			"",
			"123123",
			"machine-1",
			"",
		},
	}

	dataDir := path.Join(r.cwd, "dataDir")
	err := os.Mkdir(dataDir, os.FileMode(0755))
	c.Assert(err, jc.ErrorIsNil)

	logDir := path.Join(r.cwd, "logDir")
	err = os.Mkdir(logDir, os.FileMode(0755))
	c.Assert(err, jc.ErrorIsNil)
	for _, testCase := range cases {
		machineTag, err := names.ParseTag(testCase.machineTag)
		c.Assert(err, jc.ErrorIsNil)

		configParams := agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: dataDir,
				LogDir:  logDir,
			},
			UpgradedToVersion: jujuversion.Current,
			Tag:               machineTag,
			Controller:        coretesting.ControllerTag,
			Model:             coretesting.ModelTag,
			Password:          "placeholder",
			Nonce:             "dummyNonce",
			APIAddresses:      []string{"fakeAPIAddress:12345"},
			CACert:            coretesting.CACert,
		}
		statePort := 12345
		privateAddress := "dummyPrivateAddress"
		servingInfo := controller.StateServingInfo{
			APIPort:        1234,
			StatePort:      statePort,
			Cert:           coretesting.CACert,
			CAPrivateKey:   "a ca key",
			PrivateKey:     "a key",
			SharedSecret:   "a secret",
			SystemIdentity: "an identity",
		}

		conf, err := agent.NewStateMachineConfig(configParams, servingInfo)
		c.Assert(err, jc.ErrorIsNil)
		conf.SetOldPassword(testCase.oldPassword)
		conf.SetPassword(testCase.apiPassword)

		dialInfo, err := newDialInfo(privateAddress, conf)
		if testCase.expectedError != "" {
			c.Assert(err, gc.ErrorMatches, testCase.expectedError)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(dialInfo.Username, gc.Equals, testCase.expectedUser)
			c.Assert(dialInfo.Password, gc.Equals, testCase.expectedPassword)
			c.Assert(dialInfo.Direct, gc.Equals, true)
			c.Assert(dialInfo.Addrs, gc.DeepEquals, []string{net.JoinHostPort(privateAddress, strconv.Itoa(statePort))})
		}
	}
}

// TestUpdateMongoEntries has all the testing for this function to avoid creating multiple
// mongo instances.
func (r *RestoreSuite) TestUpdateMongoEntries(c *gc.C) {
	server := &gitjujutesting.MgoInstance{}
	err := server.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer server.DestroyWithLog()
	dialInfo := server.DialInfo()
	mgoAddr := server.Addr()
	dialInfo.Addrs = []string{mgoAddr}
	err = updateMongoEntries("1234", "0", "0", dialInfo)
	c.Assert(err, gc.ErrorMatches, "cannot update machine 0 instance information: not found")

	session, err := server.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	err = session.DB("juju").C("machines").Insert(bson.M{"machineid": "0", "instanceid": "0"})
	c.Assert(err, jc.ErrorIsNil)

	query := session.DB("juju").C("machines").Find(bson.M{"machineid": "0", "instanceid": "1234"})
	n, err := query.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)

	err = updateMongoEntries("1234", "0", "0", dialInfo)
	c.Assert(err, jc.ErrorIsNil)

	query = session.DB("juju").C("machines").Find(bson.M{"machineid": "0", "instanceid": "1234"})
	n, err = query.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
}

func (r *RestoreSuite) TestNewConnection(c *gc.C) {
	server := &gitjujutesting.MgoInstance{}
	err := server.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer server.DestroyWithLog()

	ctlr := statetesting.InitializeWithArgs(c,
		statetesting.InitializeArgs{
			Owner: names.NewLocalUserTag("test-admin"),
			Clock: testclock.NewClock(coretesting.NonZeroTime()),
		})
	st := ctlr.SystemState()
	c.Assert(ctlr.Close(), jc.ErrorIsNil)

	r.PatchValue(&mongoDefaultDialOpts, mongotest.DialOpts)
	r.PatchValue(&environsGetNewPolicyFunc, func() state.NewPolicyFunc {
		return nil
	})

	newConnection, err := connectToDB(st.ControllerTag(), names.NewModelTag(st.ModelUUID()), statetesting.NewMongoInfo())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newConnection.Close(), jc.ErrorIsNil)
}

func (r *RestoreSuite) TestRunViaSSH(c *gc.C) {
	var (
		passedAddress string
		passedArgs    []string
		passedOptions *ssh.Options
	)
	fakeSSHCommand := func(address string, args []string, options *ssh.Options) *ssh.Cmd {
		passedAddress = address
		passedArgs = args
		passedOptions = options
		return ssh.Command("", []string{"ls"}, &ssh.Options{})
	}

	r.PatchValue(&sshCommand, fakeSSHCommand)
	runViaSSH("invalidAddress", "invalidScript")
	c.Assert(passedAddress, gc.Equals, "ubuntu@invalidAddress")
	c.Assert(passedArgs, gc.DeepEquals, []string{"sudo", "-n", "bash", "-c 'invalidScript'"})

	var expectedOptions ssh.Options
	expectedOptions.SetIdentities("/var/lib/juju/system-identity")
	expectedOptions.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
	expectedOptions.SetKnownHostsFile(os.DevNull)
	c.Assert(passedOptions, jc.DeepEquals, &expectedOptions)
}
