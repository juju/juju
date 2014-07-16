// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
	"launchpad.net/goyaml"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/upstart"
	"github.com/juju/juju/version"
)

// fileSchemePrefix is the prefix for file:// URLs.
const fileSchemePrefix = "file://"

// MachineConfig represents initialization information for a new juju machine.
type MachineConfig struct {
	// Bootstrap specifies whether the new machine is the bootstrap
	// machine. When this is true, StateServingInfo should be set
	// and filled out.
	Bootstrap bool

	// StateServingInfo holds the information for serving the state.
	// This must only be set if the Bootstrap field is true
	// (state servers started subsequently will acquire their serving info
	// from another server)
	StateServingInfo *params.StateServingInfo

	// MongoInfo holds the means for the new instance to communicate with the
	// juju state database. Unless the new machine is running a state server
	// (StateServer is set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	MongoInfo *authentication.MongoInfo

	// APIInfo holds the means for the new instance to communicate with the
	// juju state API. Unless the new machine is running a state server (StateServer is
	// set), there must be at least one state server address supplied.
	// The entity name must match that of the machine being started,
	// or be empty when starting a state server.
	APIInfo *api.Info

	// InstanceId is the instance ID of the machine being initialised.
	// This is required when bootstrapping, and ignored otherwise.
	InstanceId instance.Id

	// HardwareCharacteristics contains the harrdware characteristics of
	// the machine being initialised. This optional, and is only used by
	// the bootstrap agent during state initialisation.
	HardwareCharacteristics *instance.HardwareCharacteristics

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// Tools is juju tools to be used on the new machine.
	Tools *coretools.Tools

	// DataDir holds the directory that juju state will be put in the new
	// machine.
	DataDir string

	// LogDir holds the directory that juju logs will be written to.
	LogDir string

	// Jobs holds what machine jobs to run.
	Jobs []params.MachineJob

	// CloudInitOutputLog specifies the path to the output log for cloud-init.
	// The directory containing the log file must already exist.
	CloudInitOutputLog string

	// MachineId identifies the new machine.
	MachineId string

	// MachineContainerType specifies the type of container that the machine
	// is.  If the machine is not a container, then the type is "".
	MachineContainerType instance.ContainerType

	// Networks holds a list of networks the machine should be on.
	Networks []string

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

	// WARNING: this is only set if the machine being configured is
	// a state server node.
	//
	// Config holds the initial environment configuration.
	Config *config.Config

	// Constraints holds the initial environment constraints.
	Constraints constraints.Value

	// DisableSSLHostnameVerification can be set to true to tell cloud-init
	// that it shouldn't verify SSL certificates
	DisableSSLHostnameVerification bool

	// SystemPrivateSSHKey is created at bootstrap time and recorded on every
	// node that has an API server. At this stage, that is any machine where
	// StateServer (member above) is set to true.
	SystemPrivateSSHKey string

	// DisablePackageCommands is a flag that specifies whether to suppress
	// the addition of package management commands.
	DisablePackageCommands bool

	// MachineAgentServiceName is the Upstart service name for the Juju machine agent.
	MachineAgentServiceName string

	// ProxySettings define normal http, https and ftp proxies.
	ProxySettings proxy.Settings

	// AptProxySettings define the http, https and ftp proxy settings to use
	// for apt, which may or may not be the same as the normal ProxySettings.
	AptProxySettings proxy.Settings

	// PreferIPv6 mirrors the value of prefer-ipv6 environment setting
	// and when set IPv6 addresses for connecting to the API/state
	// servers will be preferred over IPv4 ones.
	PreferIPv6 bool
}

func base64yaml(m *config.Config) string {
	data, err := goyaml.Marshal(m.AllAttrs())
	if err != nil {
		// can't happen, these values have been validated a number of times
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

// Configure updates the provided cloudinit.Config with
// configuration to initialize a Juju machine agent.
func Configure(cfg *MachineConfig, c *cloudinit.Config) error {
	if err := ConfigureBasic(cfg, c); err != nil {
		return err
	}
	return ConfigureJuju(cfg, c)
}

// NonceFile is written by cloud-init as the last thing it does.
// The file will contain the machine's nonce. The filename is
// relative to the Juju data-dir.
const NonceFile = "nonce.txt"

// ConfigureBasic updates the provided cloudinit.Config with
// basic configuration to initialise an OS image, such that it can
// be connected to via SSH, and log to a standard location.
//
// Any potentially failing operation should not be added to the
// configuration, but should instead be done in ConfigureJuju.
//
// Note: we don't do apt update/upgrade here so as not to have to wait on
// apt to finish when performing the second half of image initialisation.
// Doing it later brings the benefit of feedback in the face of errors,
// but adds to the running time of initialisation due to lack of activity
// between image bringup and start of agent installation.
func ConfigureBasic(cfg *MachineConfig, c *cloudinit.Config) error {
	c.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	c.AddSSHAuthorizedKeys(cfg.AuthorizedKeys)
	c.SetOutput(cloudinit.OutAll, "| tee -a "+cfg.CloudInitOutputLog, "")
	// Create a file in a well-defined location containing the machine's
	// nonce. The presence and contents of this file will be verified
	// during bootstrap.
	//
	// Note: this must be the last runcmd we do in ConfigureBasic, as
	// the presence of the nonce file is used to gate the remainder
	// of synchronous bootstrap.
	noncefile := path.Join(cfg.DataDir, NonceFile)
	c.AddFile(noncefile, cfg.MachineNonce, 0644)
	return nil
}

// AddAptCommands update the cloudinit.Config instance with the necessary
// packages, the request to do the apt-get update/upgrade on boot, and adds
// the apt proxy settings if there are any.
func AddAptCommands(proxySettings proxy.Settings, c *cloudinit.Config) {
	// Bring packages up-to-date.
	c.SetAptUpdate(true)
	c.SetAptUpgrade(true)
	c.SetAptGetWrapper("eatmydata")

	c.AddPackage("curl")
	c.AddPackage("cpu-checker")
	// TODO(axw) 2014-07-02 #1277359
	// Don't install bridge-utils in cloud-init;
	// leave it to the networker worker.
	c.AddPackage("bridge-utils")
	c.AddPackage("rsyslog-gnutls")

	// Write out the apt proxy settings
	if (proxySettings != proxy.Settings{}) {
		filename := apt.ConfFile
		c.AddBootCmd(fmt.Sprintf(
			`[ -f %s ] || (printf '%%s\n' %s > %s)`,
			filename,
			shquote(apt.ProxyContent(proxySettings)),
			filename))
	}
}

// ConfigureJuju updates the provided cloudinit.Config with configuration
// to initialise a Juju machine agent.
func ConfigureJuju(cfg *MachineConfig, c *cloudinit.Config) error {
	if err := verifyConfig(cfg); err != nil {
		return err
	}

	// Initialise progress reporting. We need to do separately for runcmd
	// and (possibly, below) for bootcmd, as they may be run in different
	// shell sessions.
	initProgressCmd := cloudinit.InitProgressCmd()
	c.AddRunCmd(initProgressCmd)

	// If we're doing synchronous bootstrap or manual provisioning, then
	// ConfigureBasic won't have been invoked; thus, the output log won't
	// have been set. We don't want to show the log to the user, so simply
	// append to the log file rather than teeing.
	if stdout, _ := c.Output(cloudinit.OutAll); stdout == "" {
		c.SetOutput(cloudinit.OutAll, ">> "+cfg.CloudInitOutputLog, "")
		c.AddBootCmd(initProgressCmd)
		c.AddBootCmd(cloudinit.LogProgressCmd("Logging to %s on remote host", cfg.CloudInitOutputLog))
	}

	if !cfg.DisablePackageCommands {
		AddAptCommands(cfg.AptProxySettings, c)
	}

	// Write out the normal proxy settings so that the settings are
	// sourced by bash, and ssh through that.
	c.AddScripts(
		// We look to see if the proxy line is there already as
		// the manual provider may have had it aleady. The ubuntu
		// user may not exist (local provider only).
		`([ ! -e /home/ubuntu/.profile ] || grep -q '.juju-proxy' /home/ubuntu/.profile) || ` +
			`printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> /home/ubuntu/.profile`)
	if (cfg.ProxySettings != proxy.Settings{}) {
		exportedProxyEnv := cfg.ProxySettings.AsScriptEnvironment()
		c.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		c.AddScripts(
			fmt.Sprintf(
				`[ -e /home/ubuntu ] && (printf '%%s\n' %s > /home/ubuntu/.juju-proxy && chown ubuntu:ubuntu /home/ubuntu/.juju-proxy)`,
				shquote(cfg.ProxySettings.AsScriptEnvironment())))
	}

	// Make the lock dir and change the ownership of the lock dir itself to
	// ubuntu:ubuntu from root:root so the juju-run command run as the ubuntu
	// user is able to get access to the hook execution lock (like the uniter
	// itself does.)
	lockDir := path.Join(cfg.DataDir, "locks")
	c.AddScripts(
		fmt.Sprintf("mkdir -p %s", lockDir),
		// We only try to change ownership if there is an ubuntu user
		// defined, and we determine this by the existance of the home dir.
		fmt.Sprintf("[ -e /home/ubuntu ] && chown ubuntu:ubuntu %s", lockDir),
		fmt.Sprintf("mkdir -p %s", cfg.LogDir),
		fmt.Sprintf("chown syslog:adm %s", cfg.LogDir),
	)

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	var copyCmd string
	if strings.HasPrefix(cfg.Tools.URL, fileSchemePrefix) {
		copyCmd = fmt.Sprintf("cp %s $bin/tools.tar.gz", shquote(cfg.Tools.URL[len(fileSchemePrefix):]))
	} else {
		curlCommand := "curl -sSfw 'tools from %{url_effective} downloaded: HTTP %{http_code}; time %{time_total}s; size %{size_download} bytes; speed %{speed_download} bytes/s '"
		if cfg.DisableSSLHostnameVerification {
			curlCommand += " --insecure"
		}
		copyCmd = fmt.Sprintf("%s -o $bin/tools.tar.gz %s", curlCommand, shquote(cfg.Tools.URL))
		c.AddRunCmd(cloudinit.LogProgressCmd("Fetching tools: %s", copyCmd))
	}
	toolsJson, err := json.Marshal(cfg.Tools)
	if err != nil {
		return err
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

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.NewMachineTag(cfg.MachineId)
	_, err = cfg.addAgentInfo(c, machineTag)
	if err != nil {
		return err
	}

	// Add the cloud archive cloud-tools pocket to apt sources
	// for series that need it. This gives us up-to-date LXC,
	// MongoDB, and other infrastructure.
	if !cfg.DisablePackageCommands {
		series := cfg.Tools.Version.Series
		MaybeAddCloudArchiveCloudTools(c, series)
	}

	if cfg.Bootstrap {
		cons := cfg.Constraints.String()
		if cons != "" {
			cons = " --constraints " + shquote(cons)
		}
		var hardware string
		if cfg.HardwareCharacteristics != nil {
			if hardware = cfg.HardwareCharacteristics.String(); hardware != "" {
				hardware = " --hardware " + shquote(hardware)
			}
		}
		c.AddRunCmd(cloudinit.LogProgressCmd("Bootstrapping Juju machine agent"))
		c.AddScripts(
			// The bootstrapping is always run with debug on.
			cfg.jujuTools() + "/jujud bootstrap-state" +
				" --data-dir " + shquote(cfg.DataDir) +
				" --env-config " + shquote(base64yaml(cfg.Config)) +
				" --instance-id " + shquote(string(cfg.InstanceId)) +
				hardware +
				cons +
				" --debug",
		)
	}

	return cfg.addMachineAgentToBoot(c, machineTag.String(), cfg.MachineId)
}

func (cfg *MachineConfig) dataFile(name string) string {
	return path.Join(cfg.DataDir, name)
}

func (cfg *MachineConfig) agentConfig(tag names.Tag) (agent.ConfigSetter, error) {
	// TODO for HAState: the stateHostAddrs and apiHostAddrs here assume that
	// if the machine is a stateServer then to use localhost.  This may be
	// sufficient, but needs thought in the new world order.
	var password string
	if cfg.MongoInfo == nil {
		password = cfg.APIInfo.Password
	} else {
		password = cfg.MongoInfo.Password
	}
	configParams := agent.AgentConfigParams{
		DataDir:           cfg.DataDir,
		LogDir:            cfg.LogDir,
		Jobs:              cfg.Jobs,
		Tag:               tag,
		UpgradedToVersion: version.Current.Number,
		Password:          password,
		Nonce:             cfg.MachineNonce,
		StateAddresses:    cfg.stateHostAddrs(),
		APIAddresses:      cfg.apiHostAddrs(),
		CACert:            cfg.MongoInfo.CACert,
		Values:            cfg.AgentEnvironment,
		PreferIPv6:        cfg.PreferIPv6,
	}
	if !cfg.Bootstrap {
		return agent.NewAgentConfig(configParams)
	}
	return agent.NewStateMachineConfig(configParams, *cfg.StateServingInfo)
}

// addAgentInfo adds agent-required information to the agent's directory
// and returns the agent directory name.
func (cfg *MachineConfig) addAgentInfo(c *cloudinit.Config, tag names.Tag) (agent.Config, error) {
	acfg, err := cfg.agentConfig(tag)
	if err != nil {
		return nil, err
	}
	acfg.SetValue(agent.AgentServiceName, cfg.MachineAgentServiceName)
	cmds, err := acfg.WriteCommands()
	if err != nil {
		return nil, errors.Annotate(err, "failed to write commands")
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

	name := cfg.MachineAgentServiceName
	conf := upstart.MachineAgentUpstartService(name, toolsDir, cfg.DataDir, cfg.LogDir, tag, machineId, nil)
	cmds, err := conf.InstallCommands()
	if err != nil {
		return errors.Annotatef(err, "cannot make cloud-init upstart script for the %s agent", tag)
	}
	c.AddRunCmd(cloudinit.LogProgressCmd("Starting Juju machine agent (%s)", name))
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
	if cfg.Bootstrap {
		if cfg.PreferIPv6 {
			hosts = append(hosts, net.JoinHostPort("::1", strconv.Itoa(cfg.StateServingInfo.StatePort)))
		} else {
			hosts = append(hosts, net.JoinHostPort("localhost", strconv.Itoa(cfg.StateServingInfo.StatePort)))
		}
	}
	if cfg.MongoInfo != nil {
		hosts = append(hosts, cfg.MongoInfo.Addrs...)
	}
	return hosts
}

func (cfg *MachineConfig) apiHostAddrs() []string {
	var hosts []string
	if cfg.Bootstrap {
		if cfg.PreferIPv6 {
			hosts = append(hosts, net.JoinHostPort("::1", strconv.Itoa(cfg.StateServingInfo.APIPort)))
		} else {
			hosts = append(hosts, net.JoinHostPort("localhost", strconv.Itoa(cfg.StateServingInfo.APIPort)))
		}
	}
	if cfg.APIInfo != nil {
		hosts = append(hosts, cfg.APIInfo.Addrs...)
	}
	return hosts
}

const CanonicalCloudArchiveSigningKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: SKS 1.1.4
Comment: Hostname: keyserver.ubuntu.com

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
p/+af/HU1smBrOfIeRoxb8jQoHu3
=xg4S
-----END PGP PUBLIC KEY BLOCK-----`

// MaybeAddCloudArchiveCloudTools adds the cloud-archive cloud-tools
// pocket to apt sources, if the series requires it.
func MaybeAddCloudArchiveCloudTools(c *cloudinit.Config, series string) {
	if series != "precise" {
		// Currently only precise; presumably we'll
		// need to add each LTS in here as they're
		// added to the cloud archive.
		return
	}
	const url = "http://ubuntu-cloud.archive.canonical.com/ubuntu"
	name := fmt.Sprintf("deb %s %s-updates/cloud-tools main", url, series)
	prefs := &cloudinit.AptPreferences{
		Path:        cloudinit.CloudToolsPrefsPath,
		Explanation: "Pin with lower priority, not to interfere with charms",
		Package:     "*",
		Pin:         fmt.Sprintf("release n=%s-updates/cloud-tools", series),
		PinPriority: 400,
	}
	c.AddAptSource(name, CanonicalCloudArchiveSigningKey, prefs)
}

// HasNetworks returns if there are any networks set.
func (cfg *MachineConfig) HasNetworks() bool {
	return len(cfg.Networks) > 0 || cfg.Constraints.HaveNetworks()
}

func shquote(p string) string {
	return utils.ShQuote(p)
}

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func verifyConfig(cfg *MachineConfig) (err error) {
	defer errors.Maskf(&err, "invalid machine configuration")
	if !names.IsMachine(cfg.MachineId) {
		return fmt.Errorf("invalid machine id")
	}
	if cfg.DataDir == "" {
		return fmt.Errorf("missing var directory")
	}
	if cfg.LogDir == "" {
		return fmt.Errorf("missing log directory")
	}
	if len(cfg.Jobs) == 0 {
		return fmt.Errorf("missing machine jobs")
	}
	if cfg.CloudInitOutputLog == "" {
		return fmt.Errorf("missing cloud-init output log path")
	}
	if cfg.Tools == nil {
		return fmt.Errorf("missing tools")
	}
	if cfg.Tools.URL == "" {
		return fmt.Errorf("missing tools URL")
	}
	if cfg.MongoInfo == nil {
		return fmt.Errorf("missing state info")
	}
	if len(cfg.MongoInfo.CACert) == 0 {
		return fmt.Errorf("missing CA certificate")
	}
	if cfg.APIInfo == nil {
		return fmt.Errorf("missing API info")
	}
	if len(cfg.APIInfo.CACert) == 0 {
		return fmt.Errorf("missing API CA certificate")
	}
	if cfg.MachineAgentServiceName == "" {
		return fmt.Errorf("missing machine agent service name")
	}
	if cfg.Bootstrap {
		if cfg.Config == nil {
			return fmt.Errorf("missing environment configuration")
		}
		if cfg.MongoInfo.Tag != nil {
			return fmt.Errorf("entity tag must be nil when starting a state server")
		}
		if cfg.APIInfo.Tag != nil {
			return fmt.Errorf("entity tag must be nil when starting a state server")
		}
		if cfg.StateServingInfo == nil {
			return fmt.Errorf("missing state serving info")
		}
		if len(cfg.StateServingInfo.Cert) == 0 {
			return fmt.Errorf("missing state server certificate")
		}
		if len(cfg.StateServingInfo.PrivateKey) == 0 {
			return fmt.Errorf("missing state server private key")
		}
		if cfg.StateServingInfo.StatePort == 0 {
			return fmt.Errorf("missing state port")
		}
		if cfg.StateServingInfo.APIPort == 0 {
			return fmt.Errorf("missing API port")
		}
		if cfg.SystemPrivateSSHKey == "" {
			return fmt.Errorf("missing system ssh identity")
		}
		if cfg.InstanceId == "" {
			return fmt.Errorf("missing instance-id")
		}
	} else {
		if len(cfg.MongoInfo.Addrs) == 0 {
			return fmt.Errorf("missing state hosts")
		}
		if cfg.MongoInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
		if len(cfg.APIInfo.Addrs) == 0 {
			return fmt.Errorf("missing API hosts")
		}
		if cfg.APIInfo.Tag != names.NewMachineTag(cfg.MachineId) {
			return fmt.Errorf("entity tag must match started machine")
		}
		if cfg.StateServingInfo != nil {
			return fmt.Errorf("state serving info unexpectedly present")
		}
	}
	if cfg.MachineNonce == "" {
		return fmt.Errorf("missing machine nonce")
	}
	return nil
}
