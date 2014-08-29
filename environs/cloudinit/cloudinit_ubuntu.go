// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"

	agenttool "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/service/upstart"
)

const aria2Command = "aria2c"

type ubuntuConfigure struct {
	mcfg     *MachineConfig
	conf     *cloudinit.Config
	renderer cloudinit.Renderer
}

func (w *ubuntuConfigure) init() error {
	renderer, err := cloudinit.NewRenderer(w.mcfg.Series)
	if err != nil {
		return err
	}
	w.renderer = renderer
	return nil
}

// Configure updates the provided cloudinit.Config with
// configuration to initialize a Juju machine agent.
func (w *ubuntuConfigure) Configure() error {
	if err := w.ConfigureBasic(); err != nil {
		return err
	}
	return w.ConfigureJuju()
}

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
func (w *ubuntuConfigure) ConfigureBasic() error {
	w.conf.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	w.conf.AddSSHAuthorizedKeys(w.mcfg.AuthorizedKeys)
	w.conf.SetOutput(cloudinit.OutAll, "| tee -a "+w.mcfg.CloudInitOutputLog, "")
	// Create a file in a well-defined location containing the machine's
	// nonce. The presence and contents of this file will be verified
	// during bootstrap.
	//
	// Note: this must be the last runcmd we do in ConfigureBasic, as
	// the presence of the nonce file is used to gate the remainder
	// of synchronous bootstrap.
	noncefile := path.Join(w.mcfg.DataDir, NonceFile)
	w.conf.AddTextFile(noncefile, w.mcfg.MachineNonce, 0644)
	return nil
}

// ConfigureJuju updates the provided cloudinit.Config with configuration
// to initialise a Juju machine agent.
func (w *ubuntuConfigure) ConfigureJuju() error {
	if err := verifyConfig(w.mcfg); err != nil {
		return err
	}

	// Initialise progress reporting. We need to do separately for runcmd
	// and (possibly, below) for bootcmd, as they may be run in different
	// shell sessions.
	initProgressCmd := cloudinit.InitProgressCmd()
	w.conf.AddRunCmd(initProgressCmd)

	// If we're doing synchronous bootstrap or manual provisioning, then
	// ConfigureBasic won't have been invoked; thus, the output log won't
	// have been set. We don't want to show the log to the user, so simply
	// append to the log file rather than teeing.
	if stdout, _ := w.conf.Output(cloudinit.OutAll); stdout == "" {
		w.conf.SetOutput(cloudinit.OutAll, ">> "+w.mcfg.CloudInitOutputLog, "")
		w.conf.AddBootCmd(initProgressCmd)
		w.conf.AddBootCmd(cloudinit.LogProgressCmd("Logging to %s on remote host", w.mcfg.CloudInitOutputLog))
	}

	AddAptCommands(
		w.mcfg.AptProxySettings,
		w.conf,
		w.mcfg.EnableOSRefreshUpdate,
		w.mcfg.EnableOSUpgrade,
	)

	// Write out the normal proxy settings so that the settings are
	// sourced by bash, and ssh through that.
	w.conf.AddScripts(
		// We look to see if the proxy line is there already as
		// the manual provider may have had it aleady. The ubuntu
		// user may not exist (local provider only).
		`([ ! -e /home/ubuntu/.profile ] || grep -q '.juju-proxy' /home/ubuntu/.profile) || ` +
			`printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> /home/ubuntu/.profile`)
	if (w.mcfg.ProxySettings != proxy.Settings{}) {
		exportedProxyEnv := w.mcfg.ProxySettings.AsScriptEnvironment()
		w.conf.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		w.conf.AddScripts(
			fmt.Sprintf(
				`[ -e /home/ubuntu ] && (printf '%%s\n' %s > /home/ubuntu/.juju-proxy && chown ubuntu:ubuntu /home/ubuntu/.juju-proxy)`,
				shquote(w.mcfg.ProxySettings.AsScriptEnvironment())))
	}

	// Make the lock dir and change the ownership of the lock dir itself to
	// ubuntu:ubuntu from root:root so the juju-run command run as the ubuntu
	// user is able to get access to the hook execution lock (like the uniter
	// itself does.)
	lockDir := path.Join(w.mcfg.DataDir, "locks")
	w.conf.AddScripts(
		fmt.Sprintf("mkdir -p %s", lockDir),
		// We only try to change ownership if there is an ubuntu user
		// defined, and we determine this by the existance of the home dir.
		fmt.Sprintf("[ -e /home/ubuntu ] && chown ubuntu:ubuntu %s", lockDir),
		fmt.Sprintf("mkdir -p %s", w.mcfg.LogDir),
		fmt.Sprintf("chown syslog:adm %s", w.mcfg.LogDir),
	)

	w.conf.AddScripts(
		"bin="+shquote(w.mcfg.jujuTools()),
		"mkdir -p $bin",
	)

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	if strings.HasPrefix(w.mcfg.Tools.URL, fileSchemePrefix) {
		toolsData, err := ioutil.ReadFile(w.mcfg.Tools.URL[len(fileSchemePrefix):])
		if err != nil {
			return err
		}
		w.conf.AddBinaryFile(path.Join(w.mcfg.jujuTools(), "tools.tar.gz"), []byte(toolsData), 0644)
	} else {
		var copyCmd string
		// Retry indefinitely.
		aria2Command := aria2Command + " --max-tries=0 --retry-wait=3"
		if w.mcfg.Bootstrap {
			if w.mcfg.DisableSSLHostnameVerification {
				aria2Command += " --check-certificate=false"
			}
			copyCmd = fmt.Sprintf("%s -d $bin -o tools.tar.gz %s", aria2Command, shquote(w.mcfg.Tools.URL))
		} else {
			var urls []string
			for _, addr := range w.mcfg.apiHostAddrs() {
				// TODO(axw) encode env UUID in URL when EnvironTag
				// is guaranteed to be available in APIInfo.
				url := fmt.Sprintf("https://%s/tools/%s", addr, w.mcfg.Tools.Version)
				urls = append(urls, shquote(url))
			}

			// Our certificates are unusable by aria2c (invalid subject name),
			// so we must disable certificate validation. It doesn't actually
			// matter, because there is no sensitive information being transmitted
			// and we verify the tools' hash after.
			copyCmd = fmt.Sprintf(
				"%s --check-certificate=false -d $bin -o tools.tar.gz %s",
				aria2Command,
				strings.Join(urls, " "),
			)
		}
		w.conf.AddRunCmd(cloudinit.LogProgressCmd("Fetching tools: %s", copyCmd))
		w.conf.AddRunCmd(toolsDownloadCommandWithRetry(copyCmd))
	}
	toolsJson, err := json.Marshal(w.mcfg.Tools)
	if err != nil {
		return err
	}

	w.conf.AddScripts(
		fmt.Sprintf("sha256sum $bin/tools.tar.gz > $bin/juju%s.sha256", w.mcfg.Tools.Version),
		fmt.Sprintf(`grep '%s' $bin/juju%s.sha256 || (echo "Tools checksum mismatch"; exit 1)`,
			w.mcfg.Tools.SHA256, w.mcfg.Tools.Version),
		fmt.Sprintf("tar zxf $bin/tools.tar.gz -C $bin"),
		fmt.Sprintf("printf %%s %s > $bin/downloaded-tools.txt", shquote(string(toolsJson))),
	)

	// Don't remove tools tarball until after bootstrap agent
	// runs, so it has a chance to add it to its catalogue.
	defer w.conf.AddRunCmd(
		fmt.Sprintf("rm $bin/tools.tar.gz && rm $bin/juju%s.sha256", w.mcfg.Tools.Version),
	)

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.NewMachineTag(w.mcfg.MachineId)
	_, err = addAgentInfo(w.mcfg, w.conf, machineTag, w.mcfg.Tools.Version.Number)
	if err != nil {
		return err
	}

	// Add the cloud archive cloud-tools pocket to apt sources
	// for series that need it. This gives us up-to-date LXC,
	// MongoDB, and other infrastructure.
	if w.conf.AptUpdate() {
		MaybeAddCloudArchiveCloudTools(w.conf, w.mcfg.Tools.Version.Series)
	}

	if w.mcfg.Bootstrap {
		cons := w.mcfg.Constraints.String()
		if cons != "" {
			cons = " --constraints " + shquote(cons)
		}
		var hardware string
		if w.mcfg.HardwareCharacteristics != nil {
			if hardware = w.mcfg.HardwareCharacteristics.String(); hardware != "" {
				hardware = " --hardware " + shquote(hardware)
			}
		}
		w.conf.AddRunCmd(cloudinit.LogProgressCmd("Bootstrapping Juju machine agent"))
		w.conf.AddScripts(
			// The bootstrapping is always run with debug on.
			w.mcfg.jujuTools() + "/jujud bootstrap-state" +
				" --data-dir " + shquote(w.mcfg.DataDir) +
				" --env-config " + shquote(base64yaml(w.mcfg.Config)) +
				" --instance-id " + shquote(string(w.mcfg.InstanceId)) +
				hardware +
				cons +
				" --debug",
		)
	}

	return w.addMachineAgentToBoot(machineTag.String())
}

// toolsDownloadTemplate is a bash template that attempts up to 5 times
// to run the tools download command.
const toolsDownloadTemplate = `
for n in $(seq 1 5); do
    echo "Attempt $n to download tools..."
    {{.ToolsDownloadCommand}} && echo "Tools downloaded successfully." && break
    if [ $n -lt 5 ]; then
        echo "Download failed..... wait 15s"
    fi
    sleep 15
done
`

func toolsDownloadCommandWithRetry(command string) string {
	parsedTemplate := template.Must(template.New("").Parse(toolsDownloadTemplate))
	var buf bytes.Buffer
	err := parsedTemplate.Execute(&buf, map[string]interface{}{"ToolsDownloadCommand": command})
	if err != nil {
		panic(errors.Annotate(err, "tools download template error"))
	}
	return buf.String()
}

func (w *ubuntuConfigure) addMachineAgentToBoot(tag string) error {
	// Make the agent run via a symbolic link to the actual tools
	// directory, so it can upgrade itself without needing to change
	// the upstart script.
	toolsDir := agenttool.ToolsDir(w.mcfg.DataDir, tag)
	// TODO(dfc) ln -nfs, so it doesn't fail if for some reason that the target already exists
	w.conf.AddScripts(fmt.Sprintf("ln -s %v %s", w.mcfg.Tools.Version, shquote(toolsDir)))

	name := w.mcfg.MachineAgentServiceName
	conf := upstart.MachineAgentUpstartService(
		name, toolsDir, w.mcfg.DataDir, w.mcfg.LogDir, tag, w.mcfg.MachineId, nil)
	cmds, err := conf.InstallCommands()
	if err != nil {
		return errors.Annotatef(err, "cannot make cloud-init upstart script for the %s agent", tag)
	}
	w.conf.AddRunCmd(cloudinit.LogProgressCmd("Starting Juju machine agent (%s)", name))
	w.conf.AddScripts(cmds...)
	return nil
}

func (w *ubuntuConfigure) Render() ([]byte, error) {
	return w.renderer.Render(w.conf)
}

func newUbuntuConfig(mcfg *MachineConfig, conf *cloudinit.Config) (*ubuntuConfigure, error) {
	cfg := &ubuntuConfigure{
		mcfg: mcfg,
		conf: conf,
	}
	err := cfg.init()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
