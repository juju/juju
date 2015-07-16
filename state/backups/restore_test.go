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

	"github.com/juju/names"
	"github.com/juju/replicaset"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
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

func (r *RestoreSuite) createTestFiles(c *gc.C) {
	tarDirE := path.Join(r.cwd, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, jc.ErrorIsNil)

	tarDirP := path.Join(r.cwd, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, jc.ErrorIsNil)

	tarSubFile1 := path.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, jc.ErrorIsNil)
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, jc.ErrorIsNil)

	tarFile1 := path.Join(r.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, jc.ErrorIsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := path.Join(r.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, jc.ErrorIsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()
	r.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}
}

func (r *RestoreSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, jc.ErrorIsNil)
	return mongo.EnsureAdminUser(mongo.EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
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

	session := server.MustDial()
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

var yamlLines = []string{
	"# format 1.18",
	"bogus: aBogusValue",
	"tag: aTag",
	"statepassword: aStatePassword",
	"oldpassword: anOldPassword",
	"stateport: 1",
	"apiport: 2",
	"cacert: aLengthyCACert",
}

func (r *RestoreSuite) TestSetAgentAddressScript(c *gc.C) {
	testServerAddresses := []string{
		"FirstNewStateServerAddress:30303",
		"SecondNewStateServerAddress:30304",
		"ThirdNewStateServerAddress:30305",
		"FourthNewStateServerAddress:30306",
		"FiftNewStateServerAddress:30307",
		"SixtNewStateServerAddress:30308",
	}
	for _, address := range testServerAddresses {
		template := setAgentAddressScript(address)
		expectedString := fmt.Sprintf("\t\ts/- .*(:[0-9]+)/- %s\\1/\n", address)
		logger.Infof(fmt.Sprintf("Testing with address %q", address))
		c.Assert(strings.Contains(template, expectedString), gc.Equals, true)
	}
}

var caCertPEM = `
-----BEGIN CERTIFICATE-----
MIIBnTCCAUmgAwIBAgIBADALBgkqhkiG9w0BAQUwJjENMAsGA1UEChMEanVqdTEV
MBMGA1UEAxMManVqdSB0ZXN0aW5nMB4XDTEyMTExNDE0Mzg1NFoXDTIyMTExNDE0
NDM1NFowJjENMAsGA1UEChMEanVqdTEVMBMGA1UEAxMManVqdSB0ZXN0aW5nMFow
CwYJKoZIhvcNAQEBA0sAMEgCQQCCOOpn9aWKcKr2GQGtygwD7PdfNe1I9BYiPAqa
2I33F5+6PqFdfujUKvoyTJI6XG4Qo/CECaaN9smhyq9DxzMhAgMBAAGjZjBkMA4G
A1UdDwEB/wQEAwIABDASBgNVHRMBAf8ECDAGAQH/AgEBMB0GA1UdDgQWBBQQDswP
FQGeGMeTzPbHW62EZbbTJzAfBgNVHSMEGDAWgBQQDswPFQGeGMeTzPbHW62EZbbT
JzALBgkqhkiG9w0BAQUDQQAqZzN0DqUyEfR8zIanozyD2pp10m9le+ODaKZDDNfH
8cB2x26F1iZ8ccq5IC2LtQf1IKJnpTcYlLuDvW6yB96g
-----END CERTIFICATE-----
`

func (r *RestoreSuite) TestNewDialInfo(c *gc.C) {
	machineTag, err := names.ParseTag("machine-0")
	c.Assert(err, jc.ErrorIsNil)

	dataDir := path.Join(r.cwd, "dataDir")
	err = os.Mkdir(dataDir, os.FileMode(0755))
	c.Assert(err, jc.ErrorIsNil)

	logDir := path.Join(r.cwd, "logDir")
	err = os.Mkdir(logDir, os.FileMode(0755))
	c.Assert(err, jc.ErrorIsNil)

	configParams := agent.AgentConfigParams{
		DataDir:           dataDir,
		LogDir:            logDir,
		UpgradedToVersion: version.Current.Number,
		Tag:               machineTag,
		Environment:       coretesting.EnvironmentTag,
		Password:          "dummyPassword",
		Nonce:             "dummyNonce",
		StateAddresses:    []string{"fakeStateAddress:1234"},
		APIAddresses:      []string{"fakeAPIAddress:12345"},
		CACert:            caCertPEM,
	}
	statePort := 12345
	privateAddress := "dummyPrivateAddress"
	servingInfo := params.StateServingInfo{
		APIPort:        1234,
		StatePort:      statePort,
		Cert:           caCertPEM,
		CAPrivateKey:   "a ca key",
		PrivateKey:     "a key",
		SharedSecret:   "a secret",
		SystemIdentity: "an identity",
	}

	conf, err := agent.NewStateMachineConfig(configParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)

	dialInfo, err := newDialInfo(privateAddress, conf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dialInfo.Username, gc.Equals, "admin")
	c.Assert(dialInfo.Password, gc.Equals, "dummyPassword")
	c.Assert(dialInfo.Direct, gc.Equals, true)
	c.Assert(dialInfo.Addrs, gc.DeepEquals, []string{fmt.Sprintf("%s:%d", privateAddress, statePort)})
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

	session := server.MustDial()
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

	st := statetesting.Initialize(c, names.NewLocalUserTag("test-admin"), nil, nil)
	c.Assert(st.Close(), jc.ErrorIsNil)

	r.PatchValue(&mongoDefaultDialOpts, statetesting.NewDialOpts)
	r.PatchValue(&environsNewStatePolicy, func() state.Policy { return nil })
	st, err = newStateConnection(st.EnvironTag(), statetesting.NewMongoInfo())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.Close(), jc.ErrorIsNil)
}

func (r *RestoreSuite) TestRunViaSSH(c *gc.C) {
	var (
		passedAddress string
		passedArgs    []string
	)
	fakeSSHCommand := func(address string, args []string, options *ssh.Options) *ssh.Cmd {
		passedAddress = address
		passedArgs = args
		return ssh.Command("", []string{"ls"}, &ssh.Options{})
	}

	r.PatchValue(&sshCommand, fakeSSHCommand)
	runViaSSH("invalidAddress", "invalidScript")
	c.Assert(passedAddress, gc.Equals, "ubuntu@invalidAddress")
	c.Assert(passedArgs, gc.DeepEquals, []string{"sudo", "-n", "bash", "-c 'invalidScript'"})
}
