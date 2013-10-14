// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log/syslog"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

// BootstrapStateURLFile is used to communicate to the first bootstrap node
// the URL from which to obtain important state information (instance id and
// hardware characteristics). It is a transient file, only used as the node
// is bootstrapping.
const BootstrapStateURLFile = "/tmp/provider-state-url"

// fileSchemePrefix is the prefix for file:// URLs.
const fileSchemePrefix = "file://"

// MachineConfig represents initialization information for a new juju machine.
type MachineConfig struct {
	// StateServer specifies whether the new machine will run the
	// mongo and API servers.
	StateServer bool

	// StateServerCert and StateServerKey hold the state server
	// certificate and private key in PEM format; they are required when
	// StateServer is set, and ignored otherwise.
	StateServerCert []byte
	StateServerKey  []byte

	// StatePort specifies the TCP port that will be used
	// by the MongoDB server. It must be non-zero
	// if StateServer is true.
	StatePort int

	// APIPort specifies the TCP port that will be used
	// by the API server. It must be non-zero
	// if StateServer is true.
	APIPort int

	// StateInfo holds the means for the new instance to communicate with the
	// juju state. Unless the new machine is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	StateInfo *state.Info

	// APIInfo holds the means for the new instance to communicate with the
	// juju state API. Unless the new machine is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	APIInfo *api.Info

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// Tools is juju tools to be used on the new machine.
	Tools *coretools.Tools

	// DataDir holds the directory that juju state will be put in the new
	// machine.
	DataDir string

	// MachineId identifies the new machine.
	MachineId string

	// MachineContainerType specifies the type of container that the machine
	// is.  If the machine is not a container, then the type is "".
	MachineContainerType instance.ContainerType

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// AgentEnvironment defines additional configuration variables to set in
	// the machine agent config.
	AgentEnvironment map[string]string

	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value

	// StateInfoURL is the URL of a file which contains information about the state server machines.
	StateInfoURL string

	// DisableSSLHostnameVerification can be set to true to tell cloud-init
	// that it shouldn't verify SSL certificates
	DisableSSLHostnameVerification bool
}

func base64yaml(m *config.Config) string {
	data, err := goyaml.Marshal(m.AllAttrs())
	if err != nil {
		// can't happen, these values have been validated a number of times
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func New(cfg *MachineConfig) (*cloudinit.Config, error) {
	c := cloudinit.New()
	return Configure(cfg, c)
}

func Configure(cfg *MachineConfig, c *cloudinit.Config) (*cloudinit.Config, error) {
	if err := verifyConfig(cfg); err != nil {
		return nil, err
	}
	c.AddSSHAuthorizedKeys(cfg.AuthorizedKeys)
	c.AddPackage("git")
	// Perfectly reasonable to install lxc on environment instances and kvm
	// containers.
	if cfg.MachineContainerType != instance.LXC {
		c.AddPackage("lxc")
	}

	c.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
		fmt.Sprintf("mkdir -p %s", cfg.DataDir),
		"mkdir -p /var/log/juju")

	wgetCommand := "wget"
	if cfg.DisableSSLHostnameVerification {
		wgetCommand = "wget --no-check-certificate"
	}
	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	var copyCmd string
	if strings.HasPrefix(cfg.Tools.URL, fileSchemePrefix) {
		copyCmd = fmt.Sprintf("cp %s $bin/tools.tar.gz", shquote(cfg.Tools.URL[len(fileSchemePrefix):]))
	} else {
		copyCmd = fmt.Sprintf("%s --no-verbose -O $bin/tools.tar.gz %s", wgetCommand, shquote(cfg.Tools.URL))
	}
	toolsJson, err := json.Marshal(cfg.Tools)
	if err != nil {
		return nil, err
	}
	c.AddScripts(
		"bin="+shquote(cfg.jujuTools()),
		"mkdir -p $bin",
		copyCmd,
		fmt.Sprintf("sha256sum $bin/tools.tar.gz > $bin/juju%s.sha256", cfg.Tools.Version),
		fmt.Sprintf(`grep '%s' $bin/juju%s.sha256 || (echo "Tools checksum mismatch"; exit 1)`,
			cfg.Tools.SHA256, cfg.Tools.Version),
		fmt.Sprintf("tar zxf $bin/tools.tar.gz -C $bin"),
		fmt.Sprintf("rm $bin/tools.tar.gz && rm $bin/juju%s.sha256", cfg.Tools.Version),
		fmt.Sprintf("printf %%s %s > $bin/downloaded-tools.txt", shquote(string(toolsJson))),
	)

	if err := cfg.addLogging(c); err != nil {
		return nil, err
	}

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.MachineTag(cfg.MachineId)
	_, err = cfg.addAgentInfo(c, machineTag)
	if err != nil {
		return nil, err
	}

	// Add the cloud archive cloud-tools pocket to apt sources
	// for series that need it. This gives us up-to-date LXC,
	// MongoDB, and other infrastructure.
	cfg.MaybeAddCloudArchiveCloudTools(c)

	if cfg.StateServer {
		// disable the default mongodb installed by the mongodb-server package.
		c.AddBootCmd(`echo ENABLE_MONGODB="no" > /etc/default/mongodb`)

		if cfg.NeedMongoPPA() {
			const key = "" // key is loaded from PPA
			c.AddAptSource("ppa:juju/stable", key)
		}
		c.AddPackage("mongodb-server")
		certKey := string(cfg.StateServerCert) + string(cfg.StateServerKey)
		c.AddFile(cfg.dataFile("server.pem"), certKey, 0600)
		if err := cfg.addMongoToBoot(c); err != nil {
			return nil, err
		}
		// We temporarily give bootstrap-state a directory
		// of its own so that it can get the state info via the
		// same mechanism as other jujud commands.
		// TODO(rog) 2013-10-04
		// This is redundant now as jujud bootstrap
		// uses the machine agent's configuration.
		// We leave it for the time being for backward compatibility.
		acfg, err := cfg.addAgentInfo(c, "bootstrap")
		if err != nil {
			return nil, err
		}
		cons := cfg.Constraints.String()
		if cons != "" {
			cons = " --constraints " + shquote(cons)
		}
		c.AddScripts(
			fmt.Sprintf("echo %s > %s", shquote(cfg.StateInfoURL), BootstrapStateURLFile),
			// The bootstrapping is always run with debug on.
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --data-dir "+shquote(cfg.DataDir)+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				cons+
				" --debug",
			"rm -rf "+shquote(acfg.Dir()),
		)
	}

	if err := cfg.addMachineAgentToBoot(c, machineTag, cfg.MachineId); err != nil {
		return nil, err
	}

	// general options
	c.SetAptUpgrade(true)
	c.SetAptUpdate(true)
	c.SetOutput(cloudinit.OutAll, "| tee -a /var/log/cloud-init-output.log", "")
	return c, nil
}

func (cfg *MachineConfig) addLogging(c *cloudinit.Config) error {
	var configRenderer syslog.SyslogConfigRenderer
	if cfg.StateServer {
		configRenderer = syslog.NewAccumulateConfig(
			names.MachineTag(cfg.MachineId))
	} else {
		configRenderer = syslog.NewForwardConfig(
			names.MachineTag(cfg.MachineId), cfg.stateHostAddrs())
	}
	content, err := configRenderer.Render()
	if err != nil {
		return err
	}
	c.AddFile("/etc/rsyslog.d/25-juju.conf", string(content), 0600)
	c.AddRunCmd("restart rsyslog")
	return nil
}

func (cfg *MachineConfig) dataFile(name string) string {
	return path.Join(cfg.DataDir, name)
}

func (cfg *MachineConfig) agentConfig(tag string) (agent.Config, error) {
	// TODO for HAState: the stateHostAddrs and apiHostAddrs here assume that
	// if the machine is a stateServer then to use localhost.  This may be
	// sufficient, but needs thought in the new world order.
	var password string
	if cfg.StateInfo == nil {
		password = cfg.APIInfo.Password
	} else {
		password = cfg.StateInfo.Password
	}
	configParams := agent.AgentConfigParams{
		DataDir:        cfg.DataDir,
		Tag:            tag,
		Password:       password,
		Nonce:          cfg.MachineNonce,
		StateAddresses: cfg.stateHostAddrs(),
		APIAddresses:   cfg.apiHostAddrs(),
		CACert:         cfg.StateInfo.CACert,
		Values:         cfg.AgentEnvironment,
	}
	if !cfg.StateServer {
		return agent.NewAgentConfig(configParams)
	}
	return agent.NewStateMachineConfig(agent.StateMachineConfigParams{
		AgentConfigParams: configParams,
		StateServerCert:   cfg.StateServerCert,
		StateServerKey:    cfg.StateServerKey,
		StatePort:         cfg.StatePort,
		APIPort:           cfg.APIPort,
	})
}

// addAgentInfo adds agent-required information to the agent's directory
// and returns the agent directory name.
func (cfg *MachineConfig) addAgentInfo(c *cloudinit.Config, tag string) (agent.Config, error) {
	acfg, err := cfg.agentConfig(tag)
	if err != nil {
		return nil, err
	}
	cmds, err := acfg.WriteCommands()
	if err != nil {
		return nil, err
	}
	c.AddScripts(cmds...)
	return acfg, nil
}

func (cfg *MachineConfig) addMachineAgentToBoot(c *cloudinit.Config, tag, machineId string) error {
	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := agenttools.ToolsDir(cfg.DataDir, tag)
	// TODO(dfc) ln -nfs, so it doesn't fail if for some reason that the target already exists
	c.AddScripts(fmt.Sprintf("ln -s %v %s", cfg.Tools.Version, shquote(toolsDir)))

	name := "jujud-" + tag
	conf := upstart.MachineAgentUpstartService(name, toolsDir, cfg.DataDir, "/var/log/juju/", tag, machineId, nil)
	cmds, err := conf.InstallCommands()
	if err != nil {
		return fmt.Errorf("cannot make cloud-init upstart script for the %s agent: %v", tag, err)
	}
	c.AddScripts(cmds...)
	return nil
}

func (cfg *MachineConfig) addMongoToBoot(c *cloudinit.Config) error {
	dbDir := path.Join(cfg.DataDir, "db")
	c.AddScripts(
		"mkdir -p "+dbDir+"/journal",
		"chmod 0700 "+dbDir,
		// Otherwise we get three files with 100M+ each, which takes time.
		"dd bs=1M count=1 if=/dev/zero of="+dbDir+"/journal/prealloc.0",
		"dd bs=1M count=1 if=/dev/zero of="+dbDir+"/journal/prealloc.1",
		"dd bs=1M count=1 if=/dev/zero of="+dbDir+"/journal/prealloc.2",
	)

	conf := upstart.MongoUpstartService("juju-db", cfg.DataDir, dbDir, cfg.StatePort)
	cmds, err := conf.InstallCommands()
	if err != nil {
		return fmt.Errorf("cannot make cloud-init upstart script for the state database: %v", err)
	}
	c.AddScripts(cmds...)
	return nil
}

// versionDir converts a tools URL into a name
// to use as a directory for storing the tools executables in
// by using the last element stripped of its extension.
func versionDir(toolsURL string) string {
	name := path.Base(toolsURL)
	ext := path.Ext(name)
	return name[:len(name)-len(ext)]
}

func (cfg *MachineConfig) jujuTools() string {
	return agenttools.SharedToolsDir(cfg.DataDir, cfg.Tools.Version)
}

func (cfg *MachineConfig) stateHostAddrs() []string {
	var hosts []string
	if cfg.StateServer {
		hosts = append(hosts, fmt.Sprintf("localhost:%d", cfg.StatePort))
	}
	if cfg.StateInfo != nil {
		hosts = append(hosts, cfg.StateInfo.Addrs...)
	}
	return hosts
}

func (cfg *MachineConfig) apiHostAddrs() []string {
	var hosts []string
	if cfg.StateServer {
		hosts = append(hosts, fmt.Sprintf("localhost:%d", cfg.APIPort))
	}
	if cfg.APIInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

// MaybeAddCloudArchiveCloudTools adds the cloud-archive cloud-tools
// pocket to apt sources, if the series requires it.
func (cfg *MachineConfig) MaybeAddCloudArchiveCloudTools(c *cloudinit.Config) {
	series := cfg.Tools.Version.Series
	if series != "precise" {
		// Currently only precise; presumably we'll
		// need to add each LTS in here as they're
		// added to the cloud archive.
		return
	}
	const url = "http://ubuntu-cloud.archive.canonical.com/ubuntu"
	const keyid = "EC4926EA"
	const keyserver = "" // use default
	name := fmt.Sprintf("deb %s %s-updates/cloud-tools main", url, series)
	c.AddAptSourceWithKeyId(name, keyid, keyserver)
}

func (cfg *MachineConfig) NeedMongoPPA() bool {
	series := cfg.Tools.Version.Series
	// 11.10 and earlier are not supported.
	// 12.04 can get a compatible version from the cloud-archive.
	// 13.04 and later ship a compatible version in the archive.
	return series == "quantal"
}

func shquote(p string) string {
	return utils.ShQuote(p)
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func verifyConfig(cfg *MachineConfig) (err error) {
	defer utils.ErrorContextf(&err, "invalid machine configuration")
	if !names.IsMachine(cfg.MachineId) {
		return fmt.Errorf("invalid machine id")
	}
	if cfg.DataDir == "" {
		return fmt.Errorf("missing var directory")
	}
	if cfg.Tools == nil {
		return fmt.Errorf("missing tools")
	}
	if cfg.Tools.URL == "" {
		return fmt.Errorf("missing tools URL")
	}
	if cfg.StateInfo == nil {
		return fmt.Errorf("missing state info")
	}
	if len(cfg.StateInfo.CACert) == 0 {
		return fmt.Errorf("missing CA certificate")
	}
	if cfg.APIInfo == nil {
		return fmt.Errorf("missing API info")
	}
	if len(cfg.APIInfo.CACert) == 0 {
		return fmt.Errorf("missing API CA certificate")
	}
	if cfg.StateServer {
		if cfg.Config == nil {
			return fmt.Errorf("missing environment configuration")
		}
		if cfg.StateInfo.Tag != "" {
			return fmt.Errorf("entity tag must be blank when starting a state server")
		}
		if cfg.APIInfo.Tag != "" {
			return fmt.Errorf("entity tag must be blank when starting a state server")
		}
		if len(cfg.StateServerCert) == 0 {
			return fmt.Errorf("missing state server certificate")
		}
		if len(cfg.StateServerKey) == 0 {
			return fmt.Errorf("missing state server private key")
		}
		if cfg.StatePort == 0 {
			return fmt.Errorf("missing state port")
		}
		if cfg.APIPort == 0 {
			return fmt.Errorf("missing API port")
		}
	} else {
		if len(cfg.StateInfo.Addrs) == 0 {
			return fmt.Errorf("missing state hosts")
		}
		if cfg.StateInfo.Tag != names.MachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return fmt.Errorf("missing API hosts")
		}
		if cfg.APIInfo.Tag != names.MachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
	}
	if cfg.MachineNonce == "" {
		return fmt.Errorf("missing machine nonce")
	}
	return nil
}
