// Copyright 2012-2016 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/os"
	"github.com/juju/utils/proxy"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
)

var logger = loggo.GetLogger("juju.cloudconfig")

const (
	// curlCommand is the base curl command used to download tools.
	curlCommand = "curl -sSfw 'agent binaries from %{url_effective} downloaded: HTTP %{http_code}; time %{time_total}s; size %{size_download} bytes; speed %{speed_download} bytes/s '"

	// toolsDownloadWaitTime is the number of seconds to wait between
	// each iterations of download attempts.
	toolsDownloadWaitTime = 15

	// toolsDownloadTemplate is a bash template that generates a
	// bash command to cycle through a list of URLs to download tools.
	toolsDownloadTemplate = `{{$curl := .ToolsDownloadCommand}}
n=1
while true; do
{{range .URLs}}
    printf "Attempt $n to download agent binaries from %s...\n" {{shquote .}}
    {{$curl}} {{shquote .}} && echo "Agent binaries downloaded successfully." && break
{{end}}
    echo "Download failed, retrying in {{.ToolsDownloadWaitTime}}s"
    sleep {{.ToolsDownloadWaitTime}}
    n=$((n+1))
done`
)

var (
	// UbuntuGroups is the set of unix groups to add the "ubuntu" user to
	// when initializing an Ubuntu system.
	UbuntuGroups = []string{"adm", "audio", "cdrom", "dialout", "dip",
		"floppy", "netdev", "plugdev", "sudo", "video"}

	// CentOSGroups is the set of unix groups to add the "ubuntu" user to
	// when initializing a CentOS system.
	CentOSGroups = []string{"adm", "systemd-journal", "wheel"}

	// OpenSUSEGroups is the set of unix groups to add the "ubuntu" user to
	// when initializing a OpenSUSE system.
	OpenSUSEGroups = []string{"users"}
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
	if err := w.ConfigureJuju(); err != nil {
		return err
	}
	return w.ConfigureCustomOverrides()
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
	// Keep preruncmd at the beginning of any runcmd's that juju adds
	if preruncmds, ok := w.icfg.CloudInitUserData["preruncmd"].([]interface{}); ok {
		for i := len(preruncmds) - 1; i >= 0; i -= 1 {
			if cmd, ok := preruncmds[i].(string); ok {
				w.conf.PrependRunCmd(cmd)
			}
		}
	}
	w.conf.AddRunCmd(
		"set -xe", // ensure we run all the scripts or abort.
	)
	switch w.os {
	case os.Ubuntu:
		if (w.icfg.AgentVersion() != version.Binary{}) {
			initSystem, err := service.VersionInitSystem(w.icfg.Series)
			if err != nil {
				return errors.Trace(err)
			}
			w.addCleanShutdownJob(initSystem)
		}
	case os.CentOS:
		w.conf.AddScripts(
			// Mask and stop firewalld, if enabled, so it cannot start. See
			// http://pad.lv/1492066. firewalld might be missing, in which case
			// is-enabled and is-active prints an error, which is why the output
			// is surpressed.
			"systemctl is-enabled firewalld &> /dev/null && systemctl mask firewalld || true",
			"systemctl is-active firewalld &> /dev/null && systemctl stop firewalld || true",

			`sed -i "s/^.*requiretty/#Defaults requiretty/" /etc/sudoers`,
		)
		w.addCleanShutdownJob(service.InitSystemSystemd)
	case os.OpenSUSE:
		w.conf.AddScripts(
			// Mask and stop firewalld, if enabled, so it cannot start. See
			// http://pad.lv/1492066. firewalld might be missing, in which case
			// is-enabled and is-active prints an error, which is why the output
			// is surpressed.
			"systemctl is-enabled firewalld &> /dev/null && systemctl mask firewalld || true",
			"systemctl is-active firewalld &> /dev/null && systemctl stop firewalld || true",
			`sed -i "s/^.*requiretty/#Defaults requiretty/" /etc/sudoers`,
			//Scripts assume ubuntu group for ubuntu user...
			`(grep ubuntu /etc/group) || groupadd ubuntu`,
			`usermod -g ubuntu -G ubuntu,users ubuntu`,
		)
		w.addCleanShutdownJob(service.InitSystemSystemd)
	}
	SetUbuntuUser(w.conf, w.icfg.AuthorizedKeys)

	if w.icfg.Bootstrap != nil {
		// For the bootstrap machine only, we set the host keys
		// except when manually provisioning.
		icfgKeys := w.icfg.Bootstrap.InitialSSHHostKeys
		var keys cloudinit.SSHKeys
		if icfgKeys.RSA != nil {
			keys.RSA = &cloudinit.SSHKey{
				Private: icfgKeys.RSA.Private,
				Public:  icfgKeys.RSA.Public,
			}
		}
		w.conf.SetSSHKeys(keys)
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
	var user string
	switch w.os {
	case os.CentOS, os.OpenSUSE:
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

	// To keep postruncmd at the end of any runcmd's that juju adds,
	// this block must stay at the top.
	if postruncmds, ok := w.icfg.CloudInitUserData["postruncmd"].([]interface{}); ok {
		cmds := make([]string, len(postruncmds))
		for i, v := range postruncmds {
			cmds[i] = v.(string)
		}
		defer w.conf.AddScripts(cmds...)
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
		w.conf.AddBootCmd(cloudinit.LogProgressCmd("Logging to %s on the bootstrap machine", w.icfg.CloudInitOutputLog))
	}

	if w.icfg.Bootstrap != nil {
		// Before anything else, we must regenerate the SSH host keys.
		var any bool
		keys := w.icfg.Bootstrap.InitialSSHHostKeys
		if keys.RSA != nil {
			any = true
			w.conf.AddBootCmd(cloudinit.LogProgressCmd("Regenerating SSH RSA host key"))
			w.conf.AddBootCmd(`rm /etc/ssh/ssh_host_rsa_key*`)
			w.conf.AddBootCmd(`ssh-keygen -t rsa -N "" -f /etc/ssh/ssh_host_rsa_key`)
		}
		if any {
			// ssh_keys was specified in cloud-config, which will
			// disable all key generation. Generate the other keys
			// that we did not generate previously.
			w.conf.AddBootCmd(`ssh-keygen -t dsa -N "" -f /etc/ssh/ssh_host_dsa_key`)
			w.conf.AddBootCmd(`ssh-keygen -t ecdsa -N "" -f /etc/ssh/ssh_host_ecdsa_key`)
		}
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
		// the manual provider may have had it already.
		`[ -e /etc/profile.d/juju-proxy.sh ] || ` +
			`printf '\n# Added by juju\n[ -f "/etc/juju-proxy.conf" ] && . "/etc/juju-proxy.conf"\n' >> /etc/profile.d/juju-proxy.sh`)
	if (w.icfg.ProxySettings != proxy.Settings{}) {
		exportedProxyEnv := w.icfg.ProxySettings.AsScriptEnvironment()
		w.conf.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		w.conf.AddScripts(
			fmt.Sprintf(
				`(printf '%%s\n' %s > /etc/juju-proxy.conf && chmod 0644 /etc/juju-proxy.conf)`,
				shquote(w.icfg.ProxySettings.AsScriptEnvironment())))

		// Write out systemd proxy settings
		w.conf.AddScripts(fmt.Sprintf(`printf '%%s\n' %[1]s > /etc/juju-proxy-systemd.conf`,
			shquote(w.icfg.ProxySettings.AsSystemdDefaultEnv())))
	}

	if w.icfg.Controller != nil && w.icfg.Controller.PublicImageSigningKey != "" {
		keyFile := filepath.Join(agent.DefaultPaths.ConfDir, simplestreams.SimplestreamsPublicKeyFile)
		w.conf.AddRunTextFile(keyFile, w.icfg.Controller.PublicImageSigningKey, 0644)
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

	// Make a directory for the tools to live in.
	w.conf.AddScripts(
		"bin="+shquote(w.icfg.JujuTools()),
		"mkdir -p $bin",
	)

	// Fetch the tools and unarchive them into it.
	if err := w.addDownloadToolsCmds(); err != nil {
		return errors.Trace(err)
	}

	// Don't remove tools tarball until after bootstrap agent
	// runs, so it has a chance to add it to its catalogue.
	defer w.conf.AddRunCmd(
		fmt.Sprintf("rm $bin/tools.tar.gz && rm $bin/juju%s.sha256", w.icfg.AgentVersion()),
	)

	// We add the machine agent's configuration info
	// before running bootstrap-state so that bootstrap-state
	// has a chance to rerwrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.NewMachineTag(w.icfg.MachineId)
	_, err := w.addAgentInfo(machineTag)
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

	if w.icfg.Bootstrap != nil {
		if err := w.configureBootstrap(); err != nil {
			return errors.Trace(err)
		}
	}

	// Append cloudinit-userdata packages to the end of the juju created ones.
	if packagesToAdd, ok := w.icfg.CloudInitUserData["packages"].([]interface{}); ok {
		for _, v := range packagesToAdd {
			if pack, ok := v.(string); ok {
				w.conf.AddPackage(pack)
			}
		}
	}

	return w.addMachineAgentToBoot()
}

// Not all cloudinit-userdata attr are allowed to override, these attr have been
// dealt with in ConfigureBasic() and ConfigureJuju().
func isAllowedOverrideAttr(attr string) bool {
	switch attr {
	case "packages", "preruncmd", "postruncmd":
		return false
	}
	return true
}

// ConfigureCustomOverrides implements UserdataConfig.ConfigureCustomOverrides
func (w *unixConfigure) ConfigureCustomOverrides() error {
	for k, v := range w.icfg.CloudInitUserData {
		// preruncmd was handled in ConfigureBasic()
		// packages and postruncmd have been handled in ConfigureJuju()
		if isAllowedOverrideAttr(k) {
			w.conf.SetAttr(k, v)
		}
	}
	return nil
}

func (w *unixConfigure) configureBootstrap() error {
	// Add the Juju GUI to the bootstrap node.
	cleanup, err := w.setUpGUI()
	if err != nil {
		return errors.Annotate(err, "cannot set up Juju GUI")
	}
	if cleanup != nil {
		defer cleanup()
	}

	bootstrapParamsFile := path.Join(w.icfg.DataDir, "bootstrap-params")
	bootstrapParams, err := w.icfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Annotate(err, "marshalling bootstrap params")
	}
	w.conf.AddRunTextFile(bootstrapParamsFile, string(bootstrapParams), 0600)

	loggingOption := "--show-log"
	if loggo.GetLogger("").LogLevel() == loggo.DEBUG {
		// If the bootstrap command was requested with --debug, then the root
		// logger will be set to DEBUG. If it is, then we use --debug here too.
		loggingOption = "--debug"
	}
	featureFlags := featureflag.AsEnvironmentValue()
	if featureFlags != "" {
		featureFlags = fmt.Sprintf("%s=%s ", osenv.JujuFeatureFlagEnvKey, featureFlags)
	}
	bootstrapAgentArgs := []string{
		featureFlags + w.icfg.JujuTools() + "/jujud",
		"bootstrap-state",
		"--timeout", w.icfg.Bootstrap.Timeout.String(),
		"--data-dir", shquote(w.icfg.DataDir),
		loggingOption,
		shquote(bootstrapParamsFile),
	}
	w.conf.AddRunCmd(cloudinit.LogProgressCmd("Installing Juju machine agent"))
	w.conf.AddScripts(strings.Join(bootstrapAgentArgs, " "))

	return nil
}

func (w *unixConfigure) addDownloadToolsCmds() error {
	tools := w.icfg.ToolsList()[0]
	if strings.HasPrefix(tools.URL, fileSchemePrefix) {
		toolsData, err := ioutil.ReadFile(tools.URL[len(fileSchemePrefix):])
		if err != nil {
			return err
		}
		w.conf.AddRunBinaryFile(path.Join(w.icfg.JujuTools(), "tools.tar.gz"), []byte(toolsData), 0644)
	} else {
		curlCommand := curlCommand
		var urls []string
		for _, tools := range w.icfg.ToolsList() {
			urls = append(urls, tools.URL)
		}
		if w.icfg.Bootstrap != nil {
			curlCommand += " --retry 10"
			if w.icfg.DisableSSLHostnameVerification {
				curlCommand += " --insecure"
			}
		} else {
			// Allow up to 20 seconds for curl to make a connection. This prevents
			// slow/broken routes from holding up others.
			//
			// TODO(axw) 2017-02-14 #1654943
			// When we model spaces everywhere, we should give
			// priority to the URLs that we know are accessible
			// based on space overlap.
			curlCommand += " --connect-timeout 20"

			// Don't go through the proxy when downloading tools from the controllers
			curlCommand += ` --noproxy "*"`

			// Our API server certificates are unusable by curl (invalid subject name),
			// so we must disable certificate validation. It doesn't actually
			// matter, because there is no sensitive information being transmitted
			// and we verify the tools' hash after.
			curlCommand += " --insecure"
		}
		curlCommand += " -o $bin/tools.tar.gz"
		w.conf.AddRunCmd(cloudinit.LogProgressCmd("Fetching Juju agent version %s for %s", tools.Version.Number, tools.Version.Arch))
		logger.Infof("Fetching agent: %s <%s>", curlCommand, urls)
		w.conf.AddRunCmd(toolsDownloadCommand(curlCommand, urls))
	}

	w.conf.AddScripts(
		fmt.Sprintf("sha256sum $bin/tools.tar.gz > $bin/juju%s.sha256", tools.Version),
		fmt.Sprintf(`grep '%s' $bin/juju%s.sha256 || (echo "Tools checksum mismatch"; exit 1)`,
			tools.SHA256, tools.Version),
		"tar zxf $bin/tools.tar.gz -C $bin",
	)

	toolsJson, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	w.conf.AddScripts(
		fmt.Sprintf("printf %%s %s > $bin/downloaded-tools.txt", shquote(string(toolsJson))),
	)

	return nil
}

// setUpGUI fetches the Juju GUI archive and save it to the controller.
// The returned clean up function must be called when the bootstrapping
// process is completed.
func (w *unixConfigure) setUpGUI() (func(), error) {
	if w.icfg.Bootstrap.GUI == nil {
		// No GUI archives were found on simplestreams, and no development
		// GUI path has been passed with the JUJU_GUI environment variable.
		return nil, nil
	}
	u, err := url.Parse(w.icfg.Bootstrap.GUI.URL)
	if err != nil {
		return nil, errors.Annotate(err, "cannot parse Juju GUI URL")
	}
	guiJson, err := json.Marshal(w.icfg.Bootstrap.GUI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	guiDir := w.icfg.GUITools()
	w.conf.AddScripts(
		"gui="+shquote(guiDir),
		"mkdir -p $gui",
	)
	if u.Scheme == "file" {
		// Upload the GUI from a local archive file.
		guiData, err := ioutil.ReadFile(filepath.FromSlash(u.Path))
		if err != nil {
			return nil, errors.Annotate(err, "cannot read Juju GUI archive")
		}
		w.conf.AddRunBinaryFile(path.Join(guiDir, "gui.tar.bz2"), guiData, 0644)
	} else {
		// Download the GUI from simplestreams.
		command := "curl -sSf -o $gui/gui.tar.bz2 --retry 10"
		if w.icfg.DisableSSLHostnameVerification {
			command += " --insecure"
		}
		command += " " + shquote(u.String())
		// A failure in fetching the Juju GUI archive should not prevent the
		// model to be bootstrapped. Better no GUI than no Juju at all.
		command += " || echo Unable to retrieve Juju GUI"
		w.conf.AddRunCmd(command)
	}
	w.conf.AddScripts(
		"[ -f $gui/gui.tar.bz2 ] && sha256sum $gui/gui.tar.bz2 > $gui/jujugui.sha256",
		fmt.Sprintf(
			`[ -f $gui/jujugui.sha256 ] && (grep '%s' $gui/jujugui.sha256 && printf %%s %s > $gui/downloaded-gui.txt || echo Juju GUI checksum mismatch)`,
			w.icfg.Bootstrap.GUI.SHA256, shquote(string(guiJson))),
	)
	return func() {
		// Don't remove the GUI archive until after bootstrap agent runs,
		// so it has a chance to add it to its catalogue.
		w.conf.AddRunCmd("rm -f $gui/gui.tar.bz2 $gui/jujugui.sha256 $gui/downloaded-gui.txt")
	}, nil

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
		"ToolsDownloadWaitTime": toolsDownloadWaitTime,
		"URLs":                  urls,
	})
	if err != nil {
		panic(errors.Annotate(err, "agent binaries download template error"))
	}
	return buf.String()
}
