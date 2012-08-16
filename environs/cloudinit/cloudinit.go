package cloudinit

import (
	"fmt"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/goyaml"
	"path"
	"strings"
	"encoding/base64"
)

// TODO(dfc) duplicated from environs/ec2

const zkPort = 2181

var zkPortSuffix = fmt.Sprintf(":%d", zkPort)

// MachineConfig represents initialization information for a new juju machine.
// Creation of cloudinit data from this struct is largely provider-independent,
// but we'll keep it internal until we need to factor it out.
type MachineConfig struct {
	// Provisioner specifies whether the new machine will run a provisioning agent.
	Provisioner bool

	// ZooKeeper specifies whether the new machine will run a zookeeper instance.
	ZooKeeper bool

	// InstanceIdAccessor holds bash code that evaluates to the current instance id.
	InstanceIdAccessor string

	// ProviderType identifies the provider type so the host
	// knows which kind of provider to use.
	ProviderType string

	// StateInfo holds the means for the new instance to communicate with the
	// juju state. Unless the new machine is running zookeeper (ZooKeeper is
	// set), there must be at least one zookeeper address supplied.
	StateInfo *state.Info

	// Tools is juju tools to be used on the new machine.
	Tools *state.Tools

	// MachineId identifies the new machine. It must be non-negative.
	MachineId int

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// Config specifies a set of key/values that are passed to the bootstrap machine
	// and inserted into the state on initialisation.
	Config	map[string]interface{}
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

func base64yaml(m map[string]interface{}) string {
	data, err := goyaml.Marshal(m)
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
	c.AddPackage("libzookeeper-mt2")
	if cfg.ZooKeeper {
		c.AddPackage("default-jre-headless")
		c.AddPackage("zookeeper")
		c.AddPackage("zookeeperd")
	}

	addScripts(c,
		fmt.Sprintf("sudo mkdir -p %s", environs.VarDir),
		"sudo mkdir -p /var/log/juju")

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	addScripts(c,
		"bin="+shquote(cfg.jujuTools()),
		"mkdir -p $bin",
		fmt.Sprintf("wget -O - %s | tar xz -C $bin", shquote(cfg.Tools.URL)),
	)

	addScripts(c,
		"JUJU_ZOOKEEPER="+shquote(cfg.zookeeperHostAddrs()),
		fmt.Sprintf("JUJU_MACHINE_ID=%d", cfg.MachineId),
	)

	debugFlag := ""
	if log.Debug {
		debugFlag = " --debug"
	}

	// zookeeper scripts
	if cfg.ZooKeeper {
		addScripts(c,
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --instance-id "+cfg.InstanceIdAccessor+
				" --env-type "+shquote(cfg.ProviderType)+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				" --zookeeper-servers localhost"+zkPortSuffix+
				debugFlag,
		)
	}

	if err := addAgentScript(c, cfg, "machine", fmt.Sprintf("--machine-id %d "+debugFlag, cfg.MachineId)); err != nil {
		return nil, err
	}
	if cfg.Provisioner {
		if err := addAgentScript(c, cfg, "provisioning", debugFlag); err != nil {
			return nil, err
		}
	}

	// general options
	c.SetAptUpgrade(true)
	c.SetAptUpdate(true)
	c.SetOutput(cloudinit.OutAll, "| tee -a /var/log/cloud-init-output.log", "")
	return c, nil
}

func addAgentScript(c *cloudinit.Config, cfg *MachineConfig, name, args string) error {
	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := environs.AgentToolsDir(name)
	addScripts(c, fmt.Sprintf("ln -s $bin %s", toolsDir))
	svc := upstart.NewService(fmt.Sprintf("jujud-%s", name))
	cmd := fmt.Sprintf(
		"%s/jujud %s --zookeeper-servers '%s' --log-file /var/log/juju/%s-agent.log %s",
		toolsDir, name, cfg.zookeeperHostAddrs(), name, args,
	)
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    fmt.Sprintf("juju %s agent", name),
		Cmd:     cmd,
	}
	cmds, err := conf.InstallCommands()
	if err != nil {
		return fmt.Errorf("cannot make cloud-init %s agent upstart script: %v", name, err)
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
	return environs.ToolsDir(cfg.Tools.Binary)
}

func (cfg *MachineConfig) zookeeperHostAddrs() string {
	var hosts []string
	if cfg.ZooKeeper {
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
	if cfg.Tools == nil {
		return requiresError("tools")
	}
	if cfg.Tools.URL == "" {
		return requiresError("tools URL")
	}
	if cfg.ZooKeeper {
		if cfg.InstanceIdAccessor == "" {
			return requiresError("instance id accessor")
		}
	} else {
		if cfg.StateInfo == nil || len(cfg.StateInfo.Addrs) == 0 {
			return requiresError("zookeeper hosts")
		}
	}
	return nil
}
