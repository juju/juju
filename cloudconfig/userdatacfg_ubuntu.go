// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
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
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
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
	baseConfigure
}

// TODO(ericsnow) Move Configure to the baseConfigure type?

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
	if err := w.mcfg.VerifyConfig(); err != nil {
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
		w.mcfg.Series,
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
		"bin="+shquote(w.mcfg.JujuTools()),
		"mkdir -p $bin",
	)

	// Make a directory for the tools to live in, then fetch the
	// tools and unarchive them into it.
	if strings.HasPrefix(w.mcfg.Tools.URL, fileSchemePrefix) {
		toolsData, err := ioutil.ReadFile(w.mcfg.Tools.URL[len(fileSchemePrefix):])
		if err != nil {
			return err
		}
		w.conf.AddBinaryFile(path.Join(w.mcfg.JujuTools(), "tools.tar.gz"), []byte(toolsData), 0644)
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
			for _, addr := range w.mcfg.ApiHostAddrs() {
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
	_, err = w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
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
			w.mcfg.JujuTools() + "/jujud bootstrap-state" +
				" --data-dir " + shquote(w.mcfg.DataDir) +
				" --env-config " + shquote(base64yaml(w.mcfg.Config)) +
				" --instance-id " + shquote(string(w.mcfg.InstanceId)) +
				hardware +
				cons +
				metadataDir +
				loggingOption,
		)
	}

	return w.addMachineAgentToBoot()
}

// AddAptCommands update the cloudinit.Config instance with the necessary
// packages, the request to do the apt-get update/upgrade on boot, and adds
// the apt proxy and mirror settings if there are any.
func AddAptCommands(
	series string,
	proxySettings proxy.Settings,
	aptMirror string,
	c *cloudinit.Config,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) {
	// Check preconditions
	if c == nil {
		panic("c is nil")
	}

	// Set the APT mirror.
	c.SetAptMirror(aptMirror)

	// For LTS series which need support for the cloud-tools archive,
	// we need to enable apt-get update regardless of the environ
	// setting, otherwise bootstrap or provisioning will fail.
	if series == "precise" && !addUpdateScripts {
		addUpdateScripts = true
	}

	// Bring packages up-to-date.
	c.SetAptUpdate(addUpdateScripts)
	c.SetAptUpgrade(addUpgradeScripts)
	c.SetAptGetWrapper("eatmydata")

	// If we're not doing an update, adding these packages is
	// meaningless.
	if addUpdateScripts {
		requiredPackages := []string{
			"curl",
			"cpu-checker",
			// TODO(axw) 2014-07-02 #1277359
			// Don't install bridge-utils in cloud-init;
			// leave it to the networker worker.
			"bridge-utils",
			"rsyslog-gnutls",
			"cloud-utils",
			"cloud-image-utils",
		}

		// The required packages need to come from the correct repo.
		// For precise, that might require an explicit --target-release parameter.
		for _, pkg := range requiredPackages {
			// We cannot just pass requiredPackages below, because
			// this will generate install commands which older
			// versions of cloud-init (e.g. 0.6.3 in precise) will
			// interpret incorrectly (see bug http://pad.lv/1424777).
			cmds := apt.GetPreparePackages([]string{pkg}, series)
			if len(cmds) != 1 {
				// One package given, one command (with possibly
				// multiple args) expected.
				panic(fmt.Sprintf("expected one install command per package, got %v", cmds))
			}
			for _, p := range cmds[0] {
				// We need to add them one package at a time. Also for
				// precise where --target-release
				// precise-updates/cloud-tools is needed for some packages
				// we need to pass these as "packages", otherwise the
				// aforementioned older cloud-init will also fail to
				// interpret the command correctly.
				c.AddPackage(p)
			}
		}
	}

	// Write out the apt proxy settings
	if (proxySettings != proxy.Settings{}) {
		filename := apt.ConfFile
		c.AddBootCmd(fmt.Sprintf(
			`printf '%%s\n' %s > %s`,
			shquote(apt.ProxyContent(proxySettings)),
			filename))
	}
}

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

const CanonicalCloudArchiveSigningKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: SKS 1.1.4
Comment: Hostname: keyserver.ubuntu.com

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
p/+af/HU1smBrOfIeRoxb8jQoHu3
=xg4S
-----END PGP PUBLIC KEY BLOCK-----`
