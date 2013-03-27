package cloudinit

import (
	"encoding/base64"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/log/syslog"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/upstart"
	"path"
)

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

	// MongoPort specifies the TCP port that will be used
	// by the MongoDB server. It must be non-zero
	// if StateServer is true.
	MongoPort int

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

	// Tools is juju tools to be used on the new machine.
	Tools *state.Tools

	// MongoURL is used to retrieve the mongodb tarball.
	MongoURL string

	// DataDir holds the directory that juju state will be put in the new
	// machine.
	DataDir string

	// MachineId identifies the new machine.
	MachineId string

	// AuthorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	AuthorizedKeys string

	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value
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

	addScripts(c,
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

	debugFlag := ""
	// TODO: disable debug mode by default when the system is stable.
	if true || log.Debug {
		debugFlag = " --debug"
	}

	var syslogConfigRenderer syslog.SyslogConfigRenderer
	if cfg.StateServer {
		certKey := string(cfg.StateServerCert) + string(cfg.StateServerKey)
		addFile(c, cfg.dataFile("server.pem"), certKey, 0600)
		addScripts(c,
			"mkdir -p /opt",
			fmt.Sprintf("wget --no-verbose -O - %s | tar xz -C /opt", shquote(cfg.MongoURL)),
		)
		if err := addMongoToBoot(c, cfg); err != nil {
			return nil, err
		}
		// We temporarily give bootstrap-state a directory
		// of its own so that it can get the state info via the
		// same mechanism as other jujud commands.
		acfg, err := addAgentInfo(c, cfg, "bootstrap")
		if err != nil {
			return nil, err
		}
		addScripts(c,
			cfg.jujuTools()+"/jujud bootstrap-state"+
				" --data-dir "+shquote(cfg.DataDir)+
				" --env-config "+shquote(base64yaml(cfg.Config))+
				" --constraints "+shquote(cfg.Constraints.String())+
				debugFlag,
			"rm -rf "+shquote(acfg.Dir()),
		)
		syslogConfigRenderer = syslog.NewAccumulateConfig(
			state.MachineEntityName(cfg.MachineId))
	} else {
		syslogConfigRenderer = syslog.NewForwardConfig(
			state.MachineEntityName(cfg.MachineId), cfg.stateHostAddrs())
	}

	content, err := syslogConfigRenderer.Render()
	if err != nil {
		return nil, err
	}
	addScripts(c,
		fmt.Sprintf("cat > /etc/rsyslog.d/25-juju.conf << 'EOF'\n%sEOF\n", string(content)),
	)
	c.AddRunCmd("restart rsyslog")

	if _, err := addAgentToBoot(c, cfg, "machine",
		state.MachineEntityName(cfg.MachineId),
		fmt.Sprintf("--machine-id %s "+debugFlag, cfg.MachineId)); err != nil {
		return nil, err
	}

	// general options
	c.SetAptUpgrade(true)
	c.SetAptUpdate(true)
	c.SetOutput(cloudinit.OutAll, "| tee -a /var/log/cloud-init-output.log", "")
	return c, nil
}

func addFile(c *cloudinit.Config, filename, data string, mode uint) {
	p := shquote(filename)
	addScripts(c,
		fmt.Sprintf("echo %s > %s", shquote(data), p),
		fmt.Sprintf("chmod %o %s", mode, p),
	)
}

func (cfg *MachineConfig) dataFile(name string) string {
	return path.Join(cfg.DataDir, name)
}

func (cfg *MachineConfig) agentConfig(entityName string) *agent.Conf {
	info := *cfg.StateInfo
	apiInfo := *cfg.APIInfo
	c := &agent.Conf{
		DataDir:         cfg.DataDir,
		StateInfo:       &info,
		APIInfo:         &apiInfo,
		StateServerCert: cfg.StateServerCert,
		StateServerKey:  cfg.StateServerKey,
		MongoPort:       cfg.MongoPort,
		APIPort:         cfg.APIPort,
	}
	c.StateInfo.Addrs = cfg.stateHostAddrs()
	c.StateInfo.EntityName = entityName
	c.StateInfo.Password = ""
	c.OldPassword = cfg.StateInfo.Password

	c.APIInfo.Addrs = cfg.apiHostAddrs()
	c.APIInfo.EntityName = entityName
	c.APIInfo.Password = ""

	return c
}

// addAgentInfo adds agent-required information to the agent's directory
// and returns the agent directory name.
func addAgentInfo(c *cloudinit.Config, cfg *MachineConfig, entityName string) (*agent.Conf, error) {
	acfg := cfg.agentConfig(entityName)
	cmds, err := acfg.WriteCommands()
	if err != nil {
		return nil, err
	}
	addScripts(c, cmds...)
	return acfg, nil
}

func addAgentToBoot(c *cloudinit.Config, cfg *MachineConfig, kind, entityName, args string) (*agent.Conf, error) {
	acfg, err := addAgentInfo(c, cfg, entityName)
	if err != nil {
		return nil, err
	}

	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := agent.ToolsDir(cfg.DataDir, entityName)
	// TODO(dfc) ln -nfs, so it doesn't fail if for some reason that the target already exists
	addScripts(c, fmt.Sprintf("ln -s %v %s", cfg.Tools.Binary, shquote(toolsDir)))

	svc := upstart.NewService("jujud-" + entityName)
	logPath := fmt.Sprintf("/var/log/juju/%s.log", entityName)
	cmd := fmt.Sprintf(
		"%s/jujud %s"+
			" --log-file %s"+
			" --data-dir '%s'"+
			" %s",
		toolsDir, kind,
		logPath,
		cfg.DataDir,
		args,
	)
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    fmt.Sprintf("juju %s agent", entityName),
		Cmd:     cmd,
		Out:     logPath,
	}
	cmds, err := conf.InstallCommands()
	if err != nil {
		return nil, fmt.Errorf("cannot make cloud-init upstart script for the %s agent: %v", entityName, err)
	}
	addScripts(c, cmds...)
	return acfg, nil
}

func addMongoToBoot(c *cloudinit.Config, cfg *MachineConfig) error {
	addScripts(c,
		"mkdir -p /var/lib/juju/db/journal",
		// Otherwise we get three files with 100M+ each, which takes time.
		"dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc.0",
		"dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc.1",
		"dd bs=1M count=1 if=/dev/zero of=/var/lib/juju/db/journal/prealloc.2",
	)
	svc := upstart.NewService("juju-db")
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Cmd: "/opt/mongo/bin/mongod" +
			" --auth" +
			" --dbpath=/var/lib/juju/db" +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + shquote(cfg.dataFile("server.pem")) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(cfg.MongoPort) +
			" --noprealloc" +
			" --smallfiles",
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
	return agent.SharedToolsDir(cfg.DataDir, cfg.Tools.Binary)
}

func (cfg *MachineConfig) stateHostAddrs() []string {
	var hosts []string
	if cfg.StateServer {
		hosts = append(hosts, fmt.Sprintf("localhost:%d", cfg.MongoPort))
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
	if cfg.StateInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

func shquote(p string) string {
	return trivial.ShQuote(p)
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func verifyConfig(cfg *MachineConfig) (err error) {
	defer trivial.ErrorContextf(&err, "invalid machine configuration")
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
	if cfg.StateServer {
		if cfg.Config == nil {
			return fmt.Errorf("missing environment configuration")
		}
		if cfg.StateInfo.EntityName != "" {
			return fmt.Errorf("entity name must be blank when starting a state server")
		}
		if cfg.APIInfo.EntityName != "" {
			return fmt.Errorf("entity name must be blank when starting a state server")
		}
		if len(cfg.StateServerCert) == 0 {
			return fmt.Errorf("missing state server certificate")
		}
		if len(cfg.StateServerKey) == 0 {
			return fmt.Errorf("missing state server private key")
		}
		if cfg.MongoPort == 0 {
			return fmt.Errorf("missing mongo port")
		}
		if cfg.APIPort == 0 {
			return fmt.Errorf("missing API port")
		}
	} else {
		if len(cfg.StateInfo.Addrs) == 0 {
			return fmt.Errorf("missing state hosts")
		}
		if cfg.StateInfo.EntityName != state.MachineEntityName(cfg.MachineId) {
			return fmt.Errorf("entity name must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return fmt.Errorf("missing API hosts")
		}
		if cfg.APIInfo.EntityName != state.MachineEntityName(cfg.MachineId) {
			return fmt.Errorf("entity name must match started machine")
		}
	}
	return nil
}
