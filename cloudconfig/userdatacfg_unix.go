// Copyright 2012, 2013, 2014, 2015 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"bytes"
	"encoding/base64"
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
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/version"
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

type unixConfigure struct {
	baseConfigure
}

// TODO(ericsnow) Move Configure to the baseConfigure type?

// Configure updates the provided cloudinit.Config with
// configuration to initialize a Juju machine agent.
func (w *unixConfigure) Configure() error {
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
func (w *unixConfigure) ConfigureBasic() error {
	w.conf.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	switch w.os {
	case version.Ubuntu:
		w.conf.AddSSHAuthorizedKeys(w.icfg.AuthorizedKeys)
		if w.icfg.Tools != nil {
			initSystem, err := service.VersionInitSystem(w.icfg.Series)
			if err != nil {
				return errors.Trace(err)
			}
			w.addCleanShutdownJob(initSystem)
		}
	// On unix systems that are not ubuntu we create an ubuntu user so that we
	// are able to ssh in the machine and have all the functionality dependant
	// on having an ubuntu user there.
	// Hopefully in the future we are going to move all the distirbutions to
	// having a "juju" user
	case version.CentOS:
		script := fmt.Sprintf(initUbuntuScript, utils.ShQuote(w.icfg.AuthorizedKeys))
		w.conf.AddScripts(script)
		w.conf.AddScripts("systemctl stop firewalld")
		w.conf.AddScripts("systemctl disable firewalld")
		w.conf.AddScripts(`sed -i "s/^.*requiretty/#Defaults requiretty/" /etc/sudoers`)
		w.addCleanShutdownJob(service.InitSystemSystemd)
	}
	w.conf.SetOutput(cloudinit.OutAll, "| tee -a "+w.icfg.CloudInitOutputLog, "")
	// Create a file in a well-defined location containing the machine's
	// nonce. The presence and contents of this file will be verified
	// during bootstrap.
	//
	// Note: this must be the last runcmd we do in ConfigureBasic, as
	// the presence of the nonce file is used to gate the remainder
	// of synchronous bootstrap.
	noncefile := path.Join(w.icfg.DataDir, NonceFile)
	w.conf.AddRunTextFile(noncefile, w.icfg.MachineNonce, 0644)
	return nil
}

func (w *unixConfigure) addCleanShutdownJob(initSystem string) {
	switch initSystem {
	case service.InitSystemUpstart:
		path, contents := upstart.CleanShutdownJobPath, upstart.CleanShutdownJob
		w.conf.AddRunTextFile(path, contents, 0644)
	case service.InitSystemSystemd:
		path, contents := systemd.CleanShutdownServicePath, systemd.CleanShutdownService
		w.conf.AddRunTextFile(path, contents, 0644)
		w.conf.AddScripts(fmt.Sprintf("/bin/systemctl enable '%s'", path))
	}
}

func (w *unixConfigure) setDataDirPermissions() string {
	os, _ := version.GetOSFromSeries(w.icfg.Series)
	var user string
	switch os {
	case version.CentOS:
		user = "root"
	default:
		user = "syslog"
	}
	return fmt.Sprintf("chown %s:adm %s", user, w.icfg.LogDir)
}

// ConfigureJuju updates the provided cloudinit.Config with configuration
// to initialise a Juju machine agent.
func (w *unixConfigure) ConfigureJuju() error {
	if err := w.icfg.VerifyConfig(); err != nil {
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
		w.conf.SetOutput(cloudinit.OutAll, ">> "+w.icfg.CloudInitOutputLog, "")
		w.conf.AddBootCmd(initProgressCmd)
		w.conf.AddBootCmd(cloudinit.LogProgressCmd("Logging to %s on remote host", w.icfg.CloudInitOutputLog))
	}

	w.conf.AddPackageCommands(
		w.icfg.AptProxySettings,
		w.icfg.AptMirror,
		w.icfg.EnableOSRefreshUpdate,
		w.icfg.EnableOSUpgrade,
	)

	// Write out the normal proxy settings so that the settings are
	// sourced by bash, and ssh through that.
	w.conf.AddScripts(
		// We look to see if the proxy line is there already as
		// the manual provider may have had it already. The ubuntu
		// user may not exist (local provider only).
		`([ ! -e /home/ubuntu/.profile ] || grep -q '.juju-proxy' /home/ubuntu/.profile) || ` +
			`printf '\n# Added by juju\n[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"\n' >> /home/ubuntu/.profile`)
	if (w.icfg.ProxySettings != proxy.Settings{}) {
		exportedProxyEnv := w.icfg.ProxySettings.AsScriptEnvironment()
		w.conf.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		w.conf.AddScripts(
			fmt.Sprintf(
				`(id ubuntu &> /dev/null) && (printf '%%s\n' %s > /home/ubuntu/.juju-proxy && chown ubuntu:ubuntu /home/ubuntu/.juju-proxy)`,
				shquote(w.icfg.ProxySettings.AsScriptEnvironment())))
	}

	// Make the lock dir and change the ownership of the lock dir itself to
	// ubuntu:ubuntu from root:root so the juju-run command run as the ubuntu
	// user is able to get access to the hook execution lock (like the uniter
	// itself does.)
	lockDir := path.Join(w.icfg.DataDir, "locks")
	w.conf.AddScripts(
		fmt.Sprintf("mkdir -p %s", lockDir),
		// We only try to change ownership if there is an ubuntu user defined.
		fmt.Sprintf("(id ubuntu &> /dev/null) && chown ubuntu:ubuntu %s", lockDir),
		fmt.Sprintf("mkdir -p %s", w.icfg.LogDir),
		w.setDataDirPermissions(),
	)

	w.conf.AddScripts(
		"bin="+shquote(w.icfg.JujuTools()),
		"mkdir -p $bin",
	)

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	if strings.HasPrefix(w.icfg.Tools.URL, fileSchemePrefix) {
		toolsData, err := ioutil.ReadFile(w.icfg.Tools.URL[len(fileSchemePrefix):])
		if err != nil {
			return err
		}
		w.conf.AddRunBinaryFile(path.Join(w.icfg.JujuTools(), "tools.tar.gz"), []byte(toolsData), 0644)
	} else {
		curlCommand := curlCommand
		var urls []string
		if w.icfg.Bootstrap {
			curlCommand += " --retry 10"
			if w.icfg.DisableSSLHostnameVerification {
				curlCommand += " --insecure"
			}
			urls = append(urls, w.icfg.Tools.URL)
		} else {
			for _, addr := range w.icfg.ApiHostAddrs() {
				// TODO(axw) encode env UUID in URL when EnvironTag
				// is guaranteed to be available in APIInfo.
				url := fmt.Sprintf("https://%s/tools/%s", addr, w.icfg.Tools.Version)
				urls = append(urls, url)
			}

			// Don't go through the proxy when downloading tools from the state servers
			curlCommand += ` --noproxy "*"`

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
	toolsJson, err := json.Marshal(w.icfg.Tools)
	if err != nil {
		return err
	}

	w.conf.AddScripts(
		fmt.Sprintf("sha256sum $bin/tools.tar.gz > $bin/juju%s.sha256", w.icfg.Tools.Version),
		fmt.Sprintf(`grep '%s' $bin/juju%s.sha256 || (echo "Tools checksum mismatch"; exit 1)`,
			w.icfg.Tools.SHA256, w.icfg.Tools.Version),
		fmt.Sprintf("tar zxf $bin/tools.tar.gz -C $bin"),
		fmt.Sprintf("printf %%s %s > $bin/downloaded-tools.txt", shquote(string(toolsJson))),
	)

	// Don't remove tools tarball until after bootstrap agent
	// runs, so it has a chance to add it to its catalogue.
	defer w.conf.AddRunCmd(
		fmt.Sprintf("rm $bin/tools.tar.gz && rm $bin/juju%s.sha256", w.icfg.Tools.Version),
	)

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.NewMachineTag(w.icfg.MachineId)
	_, err = w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
	}

	// Add the cloud archive cloud-tools pocket to apt sources
	// for series that need it. This gives us up-to-date LXC,
	// MongoDB, and other infrastructure.
	// This is only done on ubuntu.
	if w.conf.SystemUpdate() && w.conf.RequiresCloudArchiveCloudTools() {
		w.conf.AddCloudArchiveCloudTools()
	}

	if w.icfg.Bootstrap {
		var metadataDir string
		if len(w.icfg.CustomImageMetadata) > 0 {
			metadataDir = path.Join(w.icfg.DataDir, "simplestreams")
			index, products, err := imagemetadata.MarshalImageMetadataJSON(w.icfg.CustomImageMetadata, nil, time.Now())
			if err != nil {
				return err
			}
			indexFile := path.Join(metadataDir, imagemetadata.IndexStoragePath())
			productFile := path.Join(metadataDir, imagemetadata.ProductMetadataStoragePath())
			w.conf.AddRunTextFile(indexFile, string(index), 0644)
			w.conf.AddRunTextFile(productFile, string(products), 0644)
			metadataDir = "  --image-metadata " + shquote(metadataDir)
		}

		cons := w.icfg.Constraints.String()
		if cons != "" {
			cons = " --constraints " + shquote(cons)
		}
		var hardware string
		if w.icfg.HardwareCharacteristics != nil {
			if hardware = w.icfg.HardwareCharacteristics.String(); hardware != "" {
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
			w.icfg.JujuTools() + "/jujud bootstrap-state" +
				" --data-dir " + shquote(w.icfg.DataDir) +
				" --env-config " + shquote(base64yaml(w.icfg.Config)) +
				" --instance-id " + shquote(string(w.icfg.InstanceId)) +
				hardware +
				cons +
				metadataDir +
				loggingOption,
		)
	}

	return w.addMachineAgentToBoot()
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

func base64yaml(m *config.Config) string {
	data, err := goyaml.Marshal(m.AllAttrs())
	if err != nil {
		// can't happen, these values have been validated a number of times
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

const initUbuntuScript = `
set -e
(id ubuntu &> /dev/null) || useradd -m ubuntu -s /bin/bash
umask 0077
temp=$(mktemp)
echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > $temp
install -m 0440 $temp /etc/sudoers.d/90-juju-ubuntu
rm $temp
su ubuntu -c 'install -D -m 0600 /dev/null ~/.ssh/authorized_keys'
export authorized_keys=%s
if [ ! -z "$authorized_keys" ]; then
    su ubuntu -c 'printf "%%s\n" "$authorized_keys" >> ~/.ssh/authorized_keys'
fi`
