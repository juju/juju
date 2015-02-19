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
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"

	agenttool "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/upstart"
)

const (
	// curlCommand is the base curl command used to download tools.
	curlCommand = "curl -sSfw 'tools from %{url_effective} downloaded: HTTP %{http_code}; time %{time_total}s; size %{size_download} bytes; speed %{speed_download} bytes/s '"

	// toolsDownloadAttempts is the number of attempts to make for
	// each tools URL when downloading tools.
	toolsDownloadAttempts = 5

	// toolsDownloadWaitTime is the number of seconds to wait between
	// each iterations of download attempts.
	toolsDownloadWaitTime = 15

	// toolsDownloadTemplate is a bash template that generates a
	// bash command to cycle through a list of URLs to download tools.
	toolsDownloadTemplate = `{{$curl := .ToolsDownloadCommand}}
for n in $(seq {{.ToolsDownloadAttempts}}); do
{{range .URLs}}
    printf "Attempt $n to download tools from %s...\n" {{shquote .}}
    {{$curl}} {{shquote .}} && echo "Tools downloaded successfully." && break
{{end}}
    if [ $n -lt {{.ToolsDownloadAttempts}} ]; then
        echo "Download failed..... wait {{.ToolsDownloadWaitTime}}s"
    fi
    sleep {{.ToolsDownloadWaitTime}}
done`
)

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
		w.mcfg.AptMirror,
		w.conf,
		w.mcfg.EnableOSRefreshUpdate,
		w.mcfg.EnableOSUpgrade,
	)

	// Write out the normal proxy settings so that the settings are
	// sourced by bash, and ssh through that.
	w.conf.AddScripts(
		// We look to see if the proxy line is there already as
		// the manual provider may have had it already. The ubuntu
		// user may not exist (local provider only).
		`([ ! -e /home/ubuntu/.profile ] || grep -q '.juju-proxy' /home/ubuntu/.profile) || ` +
			`printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> /home/ubuntu/.profile`)
	if (w.mcfg.ProxySettings != proxy.Settings{}) {
		exportedProxyEnv := w.mcfg.ProxySettings.AsScriptEnvironment()
		w.conf.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		w.conf.AddScripts(
			fmt.Sprintf(
				`(id ubuntu &> /dev/null) && (printf '%%s\n' %s > /home/ubuntu/.juju-proxy && chown ubuntu:ubuntu /home/ubuntu/.juju-proxy)`,
				shquote(w.mcfg.ProxySettings.AsScriptEnvironment())))
	}

	// Make the lock dir and change the ownership of the lock dir itself to
	// ubuntu:ubuntu from root:root so the juju-run command run as the ubuntu
	// user is able to get access to the hook execution lock (like the uniter
	// itself does.)
	lockDir := path.Join(w.mcfg.DataDir, "locks")
	w.conf.AddScripts(
		fmt.Sprintf("mkdir -p %s", lockDir),
		// We only try to change ownership if there is an ubuntu user defined.
		fmt.Sprintf("(id ubuntu &> /dev/null) && chown ubuntu:ubuntu %s", lockDir),
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
		curlCommand := curlCommand
		var urls []string
		if w.mcfg.Bootstrap {
			curlCommand += " --retry 10"
			if w.mcfg.DisableSSLHostnameVerification {
				curlCommand += " --insecure"
			}
			urls = append(urls, w.mcfg.Tools.URL)
		} else {
			for _, addr := range w.mcfg.apiHostAddrs() {
				// TODO(axw) encode env UUID in URL when EnvironTag
				// is guaranteed to be available in APIInfo.
				url := fmt.Sprintf("https://%s/tools/%s", addr, w.mcfg.Tools.Version)
				urls = append(urls, url)
			}
			// Our API server certificates are unusable by curl (invalid subject name),
			// so we must disable certificate validation. It doesn't actually
			// matter, because there is no sensitive information being transmitted
			// and we verify the tools' hash after.
			curlCommand += " --insecure"
		}
		curlCommand += " -o $bin/tools.tar.gz"
		w.conf.AddRunCmd(cloudinit.LogProgressCmd("Fetching tools: %s <%s>", curlCommand, urls))
		w.conf.AddRunCmd(toolsDownloadCommand(curlCommand, urls))
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
		var metadataDir string
		if len(w.mcfg.CustomImageMetadata) > 0 {
			metadataDir = path.Join(w.mcfg.DataDir, "simplestreams")
			index, products, err := imagemetadata.MarshalImageMetadataJSON(w.mcfg.CustomImageMetadata, nil, time.Now())
			if err != nil {
				return err
			}
			indexFile := path.Join(metadataDir, imagemetadata.IndexStoragePath())
			productFile := path.Join(metadataDir, imagemetadata.ProductMetadataStoragePath())
			w.conf.AddTextFile(indexFile, string(index), 0644)
			w.conf.AddTextFile(productFile, string(products), 0644)
			metadataDir = "  --image-metadata " + shquote(metadataDir)
		}

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
		loggingOption := " --show-log"
		// If the bootstrap command was requsted with --debug, then the root
		// logger will be set to DEBUG.  If it is, then we use --debug here too.
		if loggo.GetLogger("").LogLevel() == loggo.DEBUG {
			loggingOption = " --debug"
		}
		w.conf.AddScripts(
			// The bootstrapping is always run with debug on.
			w.mcfg.jujuTools() + "/jujud bootstrap-state" +
				" --data-dir " + shquote(w.mcfg.DataDir) +
				" --env-config " + shquote(base64yaml(w.mcfg.Config)) +
				" --instance-id " + shquote(string(w.mcfg.InstanceId)) +
				hardware +
				cons +
				metadataDir +
				loggingOption,
		)
	}

	return w.addMachineAgentToBoot(machineTag.String())
}

// toolsDownloadCommand takes a curl command minus the source URL,
// and generates a command that will cycle through the URLs until
// one succeeds.
func toolsDownloadCommand(curlCommand string, urls []string) string {
	parsedTemplate := template.Must(
		template.New("ToolsDownload").Funcs(
			template.FuncMap{"shquote": shquote},
		).Parse(toolsDownloadTemplate),
	)
	var buf bytes.Buffer
	err := parsedTemplate.Execute(&buf, map[string]interface{}{
		"ToolsDownloadCommand":  curlCommand,
		"ToolsDownloadAttempts": toolsDownloadAttempts,
		"ToolsDownloadWaitTime": toolsDownloadWaitTime,
		"URLs":                  urls,
	})
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
		name, toolsDir, w.mcfg.DataDir, w.mcfg.LogDir, tag, w.mcfg.MachineId, osenv.FeatureFlags())
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
