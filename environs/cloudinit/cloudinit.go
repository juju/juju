// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"encoding/base64"
	"fmt"
	"path/filepath"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log/syslog"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

// BootstrapStateURLFile is used to communicate to the first bootstrap node
// the URL from which to obtain important state information (instance id and
// hardware characteristics). It is a transient file, only used as the node
// is bootstrapping.
const BootstrapStateURLFile = "/tmp/provider-state-url"

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
	Tools *tools.Tools

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

	// ProviderType refers to the type of the provider that created the machine.
	ProviderType string

	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value

	// StateInfoURL is the URL of a file which contains information about the state server machines.
	StateInfoURL string
}

func addScripts(c *cloudinit.Config, scripts ...string) {
	for _, s := range scripts {
		c.AddRunCmd(s)
	}
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

	addScripts(c,
		"set -xe", // ensure we run all the scripts or abort.
		fmt.Sprintf("mkdir -p %s", cfg.DataDir),
		"mkdir -p /var/log/juju")

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	addScripts(c,
		"bin="+shquote(cfg.jujuTools()),
		"mkdir -p $bin",
		fmt.Sprintf("wget --no-verbose -O - %s | tar xz -C $bin", shquote(cfg.Tools.URL)),
		fmt.Sprintf("echo -n %s > $bin/downloaded-url.txt", shquote(cfg.Tools.URL)),
	)

	// TODO (thumper): work out how to pass the logging config to the children
	debugFlag := ""
	// TODO: disable debug mode by default when the system is stable.
	if true {
		debugFlag = " --debug"
	}

	if err := cfg.addLogging(c); err != nil {
		return nil, err
	}

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := state.MachineTag(cfg.MachineId)
	_, err := cfg.addAgentInfo(c, machineTag)
	if err != nil {
		return nil, err
	}

	if cfg.StateServer {
		if cfg.NeedMongoPPA() {
			c.AddAptSource("ppa:juju/experimental", "1024R/C8068B11")
		}
		c.AddPackage("mongodb-server")
		certKey := string(cfg.StateServerCert) + string(cfg.StateServerKey)
		addFile(c, cfg.dataFile("server.pem"), certKey, 0600)
		if err := cfg.addMongoToBoot(c); err != nil {
			return nil, err
		}
		// We temporarily give bootstrap-state a directory
		// of its own so that it can get the state info via the
		// same mechanism as other jujud commands.
		acfg, err := cfg.addAgentInfo(c, "bootstrap")
		if err != nil {
			return nil, err
		}
		addScripts(c,
			fmt.Sprintf("echo %s > %s", shquote(cfg.StateInfoURL), BootstrapStateURLFile),
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --data-dir "+shquote(cfg.DataDir)+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				" --constraints "+shquote(cfg.Constraints.String())+
				debugFlag,
			"rm -rf "+shquote(acfg.Dir()),
		)
	}

	if err := cfg.addMachineAgentToBoot(c, machineTag, cfg.MachineId, debugFlag); err != nil {
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
			state.MachineTag(cfg.MachineId))
	} else {
		configRenderer = syslog.NewForwardConfig(
			state.MachineTag(cfg.MachineId), cfg.stateHostAddrs())
	}
	content, err := configRenderer.Render()
	if err != nil {
		return err
	}
	addScripts(c,
		fmt.Sprintf("cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\n%sEOF\n", string(content)),
	)
	c.AddRunCmd("restart rsyslog")
	return nil
}

func addFile(c *cloudinit.Config, filename, data string, mode uint) {
	p := shquote(filename)
	addScripts(c,
		fmt.Sprintf("echo %s > %s", shquote(data), p),
		fmt.Sprintf("chmod %o %s", mode, p),
	)
}

func (cfg *MachineConfig) dataFile(name string) string {
	return filepath.Join(cfg.DataDir, name)
}

func (cfg *MachineConfig) agentConfig(tag string) *agent.Conf {
	info := *cfg.StateInfo
	apiInfo := *cfg.APIInfo
	c := &agent.Conf{
		DataDir:         cfg.DataDir,
		StateInfo:       &info,
		APIInfo:         &apiInfo,
		StateServerCert: cfg.StateServerCert,
		StateServerKey:  cfg.StateServerKey,
		StatePort:       cfg.StatePort,
		APIPort:         cfg.APIPort,
		MachineNonce:    cfg.MachineNonce,
	}
	c.OldPassword = cfg.StateInfo.Password

	c.StateInfo.Addrs = cfg.stateHostAddrs()
	c.StateInfo.Tag = tag
	c.StateInfo.Password = ""

	c.APIInfo.Addrs = cfg.apiHostAddrs()
	c.APIInfo.Tag = tag
	c.APIInfo.Password = ""

	return c
}

// addAgentInfo adds agent-required information to the agent's directory
// and returns the agent directory name.
func (cfg *MachineConfig) addAgentInfo(c *cloudinit.Config, tag string) (*agent.Conf, error) {
	acfg := cfg.agentConfig(tag)
	cmds, err := acfg.WriteCommands()
	if err != nil {
		return nil, err
	}
	addScripts(c, cmds...)
	return acfg, nil
}

func (cfg *MachineConfig) addMachineAgentToBoot(c *cloudinit.Config, tag, machineId, logConfig string) error {
	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := tools.ToolsDir(cfg.DataDir, tag)
	// TODO(dfc) ln -nfs, so it doesn't fail if for some reason that the target already exists
	addScripts(c, fmt.Sprintf("ln -s %v %s", cfg.Tools.Version, shquote(toolsDir)))

	name := "jujud-" + tag
	conf := upstart.MachineAgentUpstartService(name, toolsDir, cfg.DataDir, "/var/log/juju/", tag, machineId, logConfig, cfg.ProviderType)
	cmds, err := conf.InstallCommands()
	if err != nil {
		return fmt.Errorf("cannot make cloud-init upstart script for the %s agent: %v", tag, err)
	}
	addScripts(c, cmds...)
	return nil
}

func (cfg *MachineConfig) addMongoToBoot(c *cloudinit.Config) error {
	dbDir := filepath.Join(cfg.DataDir, "db")
	addScripts(c,
		"mkdir -p "+dbDir+"/journal",
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
	addScripts(c, cmds...)
	return nil
}

// versionDir converts a tools URL into a name
// to use as a directory for storing the tools executables in
// by using the last element stripped of its extension.
func versionDir(toolsURL string) string {
	name := filepath.Base(toolsURL)
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

func (cfg *MachineConfig) jujuTools() string {
	return tools.SharedToolsDir(cfg.DataDir, cfg.Tools.Version)
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

func (cfg *MachineConfig) NeedMongoPPA() bool {
	series := cfg.Tools.Version.Series
	// 11.10 and earlier are not supported.
	// 13.04 and later ship a compatible version in the archive.
	return series == "precise" || series == "quantal"
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
	if !state.IsMachineId(cfg.MachineId) {
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
	if cfg.ProviderType == "" {
		return fmt.Errorf("missing provider type")
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
		if cfg.StateInfo.Tag != state.MachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return fmt.Errorf("missing API hosts")
		}
		if cfg.APIInfo.Tag != state.MachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
	}
	if cfg.MachineNonce == "" {
		return fmt.Errorf("missing machine nonce")
	}
	return nil
}
