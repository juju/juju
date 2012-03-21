package ec2

import (
	"fmt"
	"launchpad.net/juju/go/cloudinit"
	"launchpad.net/juju/go/state"
	"os/exec"
	"strings"
)

// machineConfig represents initialization information for a new juju machine.
// Creation of cloudinit data from this struct is largely provider-independent,
// but we'll keep it internal until we need to factor it out.
type machineConfig struct {
	// The new machine will run a provisioning agent.
	provisioner bool

	// The new machine will run a zookeeper instance.
	zookeeper bool

	// instanceIdAccessor holds bash code that evaluates to the current instance id.
	instanceIdAccessor string

	// AdminSecret holds a secret that will be used to authenticate to zookeeper.
	adminSecret string

	// providerType identifies the provider type so the host
	// knows which kind of provider to use.
	providerType string

	// stateInfo holds the means for the new instance to communicate with the
	// juju state. Unless the new machine is running zookeeper (Zookeeper is
	// set), there must be at least one zookeeper address supplied.
	stateInfo *state.Info

	// origin states what version of juju should be run on the instance.
	// If it is zero, a suitable default is chosen by examining the local environment.
	origin jujuOrigin

	// machineId identifies the new machine. It must be non-empty.
	machineId string

	// authorizedKeys specifies the keys that are allowed to
	// connect to the machine (see cloudinit.SSHAddAuthorizedKeys)
	// If no keys are supplied, there can be no ssh access to the node.
	// On a bootstrap machine, that is fatal. On other
	// machines it will mean that the ssh, scp and debug-hooks
	// commands cannot work.
	authorizedKeys string
}

type originKind int

// jujuOrigin represents the location of a juju distribution
type jujuOrigin struct {
	origin originKind
	url    string
}

const (
	_ originKind = iota
	originBranch
	originPPA
	originDistro
)

type requiresError string

func (e requiresError) Error() string {
	return "invalid machine configuration: missing " + string(e)
}

func addScripts(c *cloudinit.Config, scripts ...string) {
	for _, s := range scripts {
		c.AddRunCmd(s)
	}
}

func newCloudInit(cfg *machineConfig) (*cloudinit.Config, error) {
	if err := verifyConfig(cfg); err != nil {
		return nil, err
	}
	c := cloudinit.New()
	origin := cfg.origin
	if (origin == jujuOrigin{}) {
		origin = defaultOrigin()
	}

	c.AddSSHAuthorizedKeys(cfg.authorizedKeys)
	pkgs := []string{
		"bzr",
		"byobu",
		"tmux",
		"python-setuptools",
		"python-twisted",
		"python-argparse",
		"python-txaws",
		"python-zookeeper",
	}
	if cfg.zookeeper {
		pkgs = append(pkgs, []string{
			"default-jre-headless",
			"zookeeper",
			"zookeeperd",
		}...)
	}
	for _, pkg := range pkgs {
		c.AddPackage(pkg)
	}
	if cfg.origin.origin != originDistro {
		c.AddAptSource("ppa:juju/pkgs", "")
	}

	// install scripts
	if cfg.origin.origin == originDistro || cfg.origin.origin == originPPA {
		addScripts(c, "sudo apt-get -y install juju")
	} else {
		addScripts(c,
			"sudo apt-get install -y python-txzookeeper",
			"sudo mkdir -p /usr/lib/juju",
			"cd /usr/lib/juju && sudo /usr/bin/bzr co "+shquote(cfg.origin.url)+" juju",
			"cd /usr/lib/juju/juju && sudo python setup.py develop",
		)
	}
	addScripts(c,
		"sudo mkdir -p /var/lib/juju",
		"sudo mkdir -p /var/log/juju")

	// zookeeper scripts
	if cfg.zookeeper {
		addScripts(c,
			"juju-admin initialize"+
				" --instance-id="+shquote(cfg.instanceIdAccessor)+
				" --admin-identity="+shquote(makeIdentity("admin", cfg.adminSecret))+
				" --provider-type="+shquote(cfg.providerType),
		)
	}

	zookeeperHosts := shquote(cfg.zookeeperHostAddrs())

	// machine scripts
	addScripts(c, fmt.Sprintf(
		"JUJU_MACHINE_ID=%s JUJU_ZOOKEEPER=%s"+
			" python -m juju.agents.machine -n"+
			" --logfile=/var/log/juju/machine-agent.log"+
			" --pidfile=/var/run/juju/machine-agent.pid",
		shquote(cfg.machineId), zookeeperHosts))

	// provision scripts
	if cfg.provisioner {
		addScripts(c,
			"JUJU_ZOOKEEPER="+zookeeperHosts+
				" python -m juju.agents.provision -n"+
				" --logfile=/var/log/juju/provision-agent.log"+
				" --pidfile=/var/run/juju/provision-agent.pid",
		)
	}

	// general options
	c.SetAptUpgrade(true)
	c.SetAptUpdate(true)
	c.SetOutput(cloudinit.OutAll, "| tee -a /var/log/cloud-init-output.log", "")
	return c, nil
}

func (cfg *machineConfig) zookeeperHostAddrs() string {
	var hosts []string
	if cfg.zookeeper {
		hosts = append(hosts, "localhost"+zkPortSuffix)
	}
	if cfg.stateInfo != nil {
		hosts = append(hosts, cfg.stateInfo.Addrs...)
	}
	return strings.Join(hosts, ",")
}

// shquote quotes s so that when read by bash, no metacharacters
// within s will be interpreted as such.
func shquote(s string) string {
	// single-quote becomes single-quote, double-quote, single-quote, double-quote, single-quote
	return `'` + strings.Replace(s, `'`, `'"'"'`, -1) + `'`
}

func verifyConfig(cfg *machineConfig) error {
	if cfg.machineId == "" {
		return requiresError("machine id")
	}
	if cfg.providerType == "" {
		return requiresError("provider type")
	}
	if cfg.zookeeper {
		if cfg.instanceIdAccessor == "" {
			return requiresError("instance id accessor")
		}
		if cfg.adminSecret == "" {
			return requiresError("admin secret")
		}
	} else {
		if cfg.stateInfo == nil || len(cfg.stateInfo.Addrs) == 0 {
			return requiresError("zookeeper hosts")
		}
	}
	return nil
}

type lines []string

// next finds the next non-blank line in lines and returns the number of
// leading spaces and the line itself, stripped of leading spaces.
func (l *lines) next() (int, string) {
	for len(*l) > 0 {
		s := (*l)[0]
		*l = (*l)[1:]
		t := strings.TrimLeft(s, " ")
		if t != "" {
			return len(s) - len(t), t
		}
	}
	return 0, ""
}

// nextWithPrefix returns the next line from lines that has the given prefix,
// with the prefix removed.  If there is no such line, it returns the empty
// string and false.
func (l *lines) nextWithPrefix(prefix string) (string, bool) {
	for {
		_, line := l.next()
		if line == "" {
			return "", false
		}
		if strings.HasPrefix(line, prefix) {
			return line[len(prefix):], true
		}
	}
	panic("not reached")
}

var fallbackOrigin = jujuOrigin{originDistro, ""}

func parseOrigin(s string) (jujuOrigin, error) {
	switch s {
	case "distro":
		return jujuOrigin{originDistro, ""}, nil
	case "ppa":
		return jujuOrigin{originPPA, ""}, nil
	}
	if !strings.HasPrefix(s, "lp:") {
		return jujuOrigin{}, fmt.Errorf("unknown juju origin %q", s)
	}
	return jujuOrigin{originBranch, s}, nil
}

// defaultOrigin selects the best fit for running juju on cloudinit.
// It is used only if the origin is not otherwise specified
// in Config.origin.
func defaultOrigin() jujuOrigin {
	// TODO how can we (or should we?) determine if we're running from a branch?
	data, err := exec.Command("apt-cache", "policy", "juju").Output()
	if err != nil {
		// TODO log the error?
		return fallbackOrigin
	}
	return policyToOrigin(string(data))
}

func policyToOrigin(policy string) jujuOrigin {
	out := lines(strings.Split(policy, "\n"))
	_, line := out.next()
	if line == "" {
		return fallbackOrigin
	}
	if line == "N: Unable to locate package juju" {
		return jujuOrigin{originBranch, "lp:juju"}
	}

	// Find installed version.
	version, ok := out.nextWithPrefix("Installed:")
	version = strings.TrimLeft(version, " ")
	if !ok {
		return fallbackOrigin
	}
	if version == "(none)" {
		return jujuOrigin{originBranch, "lp:juju"}
	}

	_, ok = out.nextWithPrefix("Version table:")
	if !ok {
		return fallbackOrigin
	}
	// Find installed version within the table.
	_, ok = out.nextWithPrefix("*** " + version + " ")
	if !ok {
		return fallbackOrigin
	}

	firstIndent, line := out.next()
	for len(line) > 0 {
		if strings.Contains(line, "http://ppa.launchpad.net/juju/pkgs/") {
			return jujuOrigin{originPPA, ""}
		}
		var indent int
		indent, line = out.next()
		if indent != firstIndent {
			break
		}
	}
	return fallbackOrigin
}
