package cloudinit

import (
	"encoding/base64"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"path"
	"strings"
)

// TODO(dfc) duplicated from environs/ec2

const zkPort = 37017

var zkPortSuffix = fmt.Sprintf(":%d", zkPort)

// MachineConfig represents initialization information for a new juju machine.
// Creation of cloudinit data from this struct is largely provider-independent,
// but we'll keep it internal until we need to factor it out.
type MachineConfig struct {
	// Provisioner specifies whether the new machine will run a provisioning agent.
	Provisioner bool

	// StateServer specifies whether the new machine will run a ZooKeeper 
	// or MongoDB instance.
	StateServer bool

	// InstanceIdAccessor holds bash code that evaluates to the current instance id.
	InstanceIdAccessor string

	// ProviderType identifies the provider type so the host
	// knows which kind of provider to use.
	ProviderType string

	// StateInfo holds the means for the new instance to communicate with the
	// juju state. Unless the new machine is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	StateInfo *state.Info

	// Tools is juju tools to be used on the new machine.
	Tools *state.Tools

	// DataDir holds the directory that juju state will be put in the new
	// machine.
	DataDir string

	// MachineId identifies the new machine. It must be non-negative.
	MachineId int

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// Config holds the initial environment configuration.
	Config *config.Config
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
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
	if err := verifyConfig(cfg); err != nil {
		return nil, err
	}
	c := cloudinit.New()

	c.AddSSHAuthorizedKeys(cfg.AuthorizedKeys)
	c.AddPackage("git")
	if cfg.StateServer {
	}

	addScripts(c,
		fmt.Sprintf("sudo mkdir -p %s", cfg.DataDir),
		"sudo mkdir -p /var/log/juju")

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	addScripts(c,
		"bin="+shquote(cfg.jujuTools()),
		"mkdir -p $bin",
		fmt.Sprintf("wget -O - %s | tar xz -C $bin", shquote(cfg.Tools.URL)),
		fmt.Sprintf("echo -n %s > $bin/downloaded-url.txt", shquote(cfg.Tools.URL)),
	)

	debugFlag := ""
	// TODO: disable debug mode by default when the system is stable.
	if true || log.Debug {
		debugFlag = " --debug"
	}

	if cfg.StateServer {
		// TODO The public bucket must come from the environment configuration.
		b := cfg.Tools.Binary
		url := fmt.Sprintf("http://juju-dist.s3.amazonaws.com/tools/mongo-2.2.0-%s-%s.tgz", b.Series, b.Arch)
		addScripts(c,
			"mkdir -p /opt",
			fmt.Sprintf("wget -O - %s | tar xz -C /opt", shquote(url)),
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --instance-id "+cfg.InstanceIdAccessor+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				" --state-servers localhost"+zkPortSuffix+
				debugFlag,
		)
		if err := addMongoToBoot(c); err != nil {
			return nil, err
		}
		addScripts(c,
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --instance-id "+cfg.InstanceIdAccessor+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				" --state-servers localhost"+zkPortSuffix+
				debugFlag,
		)
	}

	if err := addAgentToBoot(c, cfg, "machine", fmt.Sprintf("--machine-id %d "+debugFlag, cfg.MachineId)); err != nil {
		return nil, err
	}
	if cfg.Provisioner {
		if err := addAgentToBoot(c, cfg, "provisioning", debugFlag); err != nil {
			return nil, err
		}
	}

	// general options
	c.SetAptUpgrade(true)
	c.SetAptUpdate(true)
	c.SetOutput(cloudinit.OutAll, "| tee -a /var/log/cloud-init-output.log", "")
	return c, nil
}

func addAgentToBoot(c *cloudinit.Config, cfg *MachineConfig, name, args string) error {
	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := environs.AgentToolsDir(cfg.DataDir, name)
	addScripts(c, fmt.Sprintf("ln -s %v %s", cfg.Tools.Binary, toolsDir))
	svc := upstart.NewService("jujud-" + name)
	cmd := fmt.Sprintf(
		"%s/jujud %s"+
			" --state-servers '%s'"+
			" --log-file /var/log/juju/%s-agent.log"+
			" --data-dir '%s'"+
			" %s",
		toolsDir, name,
		cfg.zookeeperHostAddrs(),
		name,
		cfg.DataDir,
		args,
	)
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    fmt.Sprintf("juju %s agent", name),
		Cmd:     cmd,
	}
	cmds, err := conf.InstallCommands()
	if err != nil {
		return fmt.Errorf("cannot make cloud-init upstart script for the %s agent: %v", name, err)
	}
	addScripts(c, cmds...)
	return nil
}

func addMongoToBoot(c *cloudinit.Config) error {
	addScripts(c, fmt.Sprintf("mkdir -p /var/lib/juju/db"))
	svc := upstart.NewService("juju-db")
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Cmd:     "/opt/mongo/bin/mongod --port 37017 --bind_ip 127.0.0.1 --dbpath=/var/lib/juju/db",
	}
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
	name := path.Base(toolsURL)
	ext := path.Ext(name)
	return name[:len(name)-len(ext)]
}

func (cfg *MachineConfig) jujuTools() string {
	return environs.ToolsDir(cfg.DataDir, cfg.Tools.Binary)
}

func (cfg *MachineConfig) zookeeperHostAddrs() string {
	var hosts []string
	if cfg.StateServer {
		hosts = append(hosts, "localhost"+zkPortSuffix)
	}
	if cfg.StateInfo != nil {
		hosts = append(hosts, cfg.StateInfo.Addrs...)
	}
	return strings.Join(hosts, ",")
}

// shquote quotes s so that when read by bash, no metacharacters
// within s will be interpreted as such.
func shquote(s string) string {
	// single-quote becomes single-quote, double-quote, single-quote, double-quote, single-quote
	return `'` + strings.Replace(s, `'`, `'"'"'`, -1) + `'`
}

func verifyConfig(cfg *MachineConfig) error {
	if cfg.MachineId < 0 {
		return fmt.Errorf("invalid machine configuration: negative machine id")
	}
	if cfg.ProviderType == "" {
		return requiresError("provider type")
	}
	if cfg.DataDir == "" {
		return requiresError("var directory")
	}
	if cfg.Tools == nil {
		return requiresError("tools")
	}
	if cfg.Tools.URL == "" {
		return requiresError("tools URL")
	}
	if cfg.StateServer {
		if cfg.InstanceIdAccessor == "" {
			return requiresError("instance id accessor")
		}
		if cfg.Config == nil {
			return requiresError("environment configuration")
		}
	} else {
		if cfg.StateInfo == nil || len(cfg.StateInfo.Addrs) == 0 {
			return requiresError("zookeeper hosts")
		}
	}
	return nil
}
