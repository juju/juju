// Copyright 2012-2016 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdos "os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.cloudconfig")

const (
	// FileNameBootstrapParams is the name of bootstrap params file.
	FileNameBootstrapParams = "bootstrap-params"

	// curlCommand is the base curl command used to download tools.
	curlCommand = "curl -sSf"

	// toolsDownloadWaitTime is the number of seconds to wait between
	// each iterations of download attempts.
	toolsDownloadWaitTime = 15

	// toolsDownloadTemplate is a bash template that generates a
	// bash command to cycle through a list of URLs to download tools.
	toolsDownloadTemplate = `{{$curl := .ToolsDownloadCommand}}
n=1
while true; do
{{range .URLs}}
    echo "Attempt $n to download agent binaries from {{shquote .}}...\n"
    {{$curl}} {{shquote .}} && echo "Agent binaries downloaded successfully." && break
{{end}}
    echo "Download failed, retrying in {{.ToolsDownloadWaitTime}}s"
    sleep {{.ToolsDownloadWaitTime}}
    n=$((n+1))
done`

	// removeServicesScript is written to /sbin and can be used to remove
	// all Juju services from a machine.
	// Once this script is run, logic to check whether such a machine is already
	// provisioned should return false and the machine can be reused as a target
	// for either bootstrap or add-machine.
	removeServicesScript = `#!/bin/bash

# WARNING
# This script will clean a host previously used to run a Juju controller/machine.
# Running this on a live installation will render Juju inoperable.

for path_to_unit in $(ls /etc/systemd/system/juju*); do
  echo "removing juju service: $path_to_unit"
  unit=$(basename "$path_to_unit")
  systemctl stop "$unit"
  systemctl disable "$unit"
  systemctl daemon-reload
  rm -f "$path_to_unit"
done

echo "removing /var/lib/juju/tools/*"
rm -rf /var/lib/juju/tools/*

echo "removing /var/lib/juju/db/*"
rm -rf /var/lib/juju/db/*

echo "removing /var/lib/juju/raft/*"
rm -rf /var/lib/juju/raft/*

echo "removing /var/run/juju/*"
rm -rf /var/run/juju/*

has_juju_db_snap=$(snap info juju-db | grep installed:)
if [ ! -z "$has_juju_db_snap" ]; then
  echo "removing juju-db snap and any persisted database data"
  snap remove --purge juju-db
fi
`
	// We look to see if the proxy line is there already as
	// the manual provider may have had it already.
	// We write this file out whether we are using the legacy proxy
	// or the juju proxy to deal with runtime changes. The proxy updater worker
	// only modifies /etc/juju-proxy.conf, so if changes are written to that file
	// we need to make sure the profile.d file exists to reflect these changes.
	// If the new juju proxies are used, the legacy proxies will not be set, and the
	// /etc/juju-proxy.conf file will be empty.
	JujuProxyProfileScript = `
if [ ! -e /etc/profile.d/juju-proxy.sh ]; then
  (
    echo
    echo '# Added by juju'
    echo
    echo '[ -f /etc/juju-proxy.conf ] && . /etc/juju-proxy.conf'
    echo
  ) >> /etc/profile.d/juju-proxy.sh
fi
`
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
			cmd, err := runCmdToString(preruncmds[i])
			if err != nil {
				return errors.Annotate(err, "invalid preruncmd")
			}
			w.conf.PrependRunCmd(cmd)
		}
	}
	w.conf.AddRunCmd(
		"set -xe", // ensure we run all the scripts or abort.
	)
	switch w.os {
	case os.CentOS:
		w.conf.AddScripts(
			// Mask and stop firewalld, if enabled, so it cannot start. See
			// http://pad.lv/1492066. firewalld might be missing, in which case
			// is-enabled and is-active prints an error, which is why the output
			// is suppressed.
			"systemctl is-enabled firewalld &> /dev/null && systemctl mask firewalld || true",
			"systemctl is-active firewalld &> /dev/null && systemctl stop firewalld || true",

			`sed -i "s/^.*requiretty/#Defaults requiretty/" /etc/sudoers`,
		)
	case os.OpenSUSE:
		w.conf.AddScripts(
			// Mask and stop firewalld, if enabled, so it cannot start. See
			// http://pad.lv/1492066. firewalld might be missing, in which case
			// is-enabled and is-active prints an error, which is why the output
			// is suppressed.
			"systemctl is-enabled firewalld &> /dev/null && systemctl mask firewalld || true",
			"systemctl is-active firewalld &> /dev/null && systemctl stop firewalld || true",
			`sed -i "s/^.*requiretty/#Defaults requiretty/" /etc/sudoers`,
			//Scripts assume ubuntu group for ubuntu user...
			`(grep ubuntu /etc/group) || groupadd ubuntu`,
			`usermod -g ubuntu -G ubuntu,users ubuntu`,
		)
	}
	SetUbuntuUser(w.conf, w.icfg.AuthorizedKeys)

	if w.icfg.Bootstrap != nil {
		// For the bootstrap machine only, we set the host keys
		// except when manually provisioning.
		icfgKeys := w.icfg.Bootstrap.InitialSSHHostKeys
		var keys cloudinit.SSHKeys
		for _, hostKey := range icfgKeys {
			keys = append(keys, cloudinit.SSHKey{
				Private:            hostKey.Private,
				Public:             hostKey.Public,
				PublicKeyAlgorithm: hostKey.PublicKeyAlgorithm,
			})
		}
		err := w.conf.SetSSHKeys(keys)
		if err != nil {
			return errors.Annotate(err, "setting ssh keys")
		}
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
			cmd, err := runCmdToString(v)
			if err != nil {
				return errors.Annotate(err, "invalid postruncmd")
			}
			cmds[i] = cmd
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

	if w.icfg.Bootstrap != nil && len(w.icfg.Bootstrap.InitialSSHHostKeys) > 0 {
		// Before anything else, we must regenerate the SSH host keys.
		// During bootstrap we provide our own keys, but to prevent keys being
		// sniffed from metadata by user applications that shouldn't have access,
		// we regenerate them.
		w.conf.AddBootCmd(cloudinit.LogProgressCmd("Regenerating SSH host keys"))
		w.conf.AddBootCmd(`rm /etc/ssh/ssh_host_*_key*`)
		w.conf.AddBootCmd(`ssh-keygen -t rsa -N "" -f /etc/ssh/ssh_host_rsa_key`)
		w.conf.AddBootCmd(`ssh-keygen -t ecdsa -N "" -f /etc/ssh/ssh_host_ecdsa_key`)
		// We drop DSA due to it not really being supported by default anymore,
		// we also softly fail on ed25519 as it may not be supported by the target
		// machine.
		w.conf.AddBootCmd(`ssh-keygen -t ed25519 -N "" -f /etc/ssh/ssh_host_ed25519_key || true`)
	}

	if err := w.conf.AddPackageCommands(
		packageManagerProxySettings{
			aptProxy:            w.icfg.AptProxySettings,
			aptMirror:           w.icfg.AptMirror,
			snapProxy:           w.icfg.SnapProxySettings,
			snapStoreAssertions: w.icfg.SnapStoreAssertions,
			snapStoreProxyID:    w.icfg.SnapStoreProxyID,
			snapStoreProxyURL:   w.icfg.SnapStoreProxyURL,
		},
		w.icfg.EnableOSRefreshUpdate,
		w.icfg.EnableOSUpgrade,
	); err != nil {
		return errors.Trace(err)
	}

	// Write out the normal proxy settings so that the settings are
	// sourced by bash, and ssh through that.
	w.conf.AddScripts(JujuProxyProfileScript)
	if w.icfg.LegacyProxySettings.HasProxySet() {
		exportedProxyEnv := w.icfg.LegacyProxySettings.AsScriptEnvironment()
		w.conf.AddScripts(strings.Split(exportedProxyEnv, "\n")...)
		w.conf.AddScripts(
			fmt.Sprintf(
				`(echo %s > /etc/juju-proxy.conf && chmod 0644 /etc/juju-proxy.conf)`,
				shquote(w.icfg.LegacyProxySettings.AsScriptEnvironment())))

		// Write out systemd proxy settings
		w.conf.AddScripts(fmt.Sprintf(`echo %[1]s > /etc/juju-proxy-systemd.conf`,
			shquote(w.icfg.LegacyProxySettings.AsSystemdDefaultEnv())))
	}

	if w.icfg.PublicImageSigningKey != "" {
		keyFile := filepath.Join(agent.DefaultPaths.ConfDir, simplestreams.SimplestreamsPublicKeyFile)
		w.conf.AddRunTextFile(keyFile, w.icfg.PublicImageSigningKey, 0644)
	}

	// Make the lock dir and change the ownership of the lock dir itself to
	// ubuntu:ubuntu from root:root so the juju-exec command run as the ubuntu
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
	// has a chance to rewrite it to change the password.
	// It would be cleaner to change bootstrap-state to
	// be responsible for starting the machine agent itself,
	// but this would not be backwardly compatible.
	machineTag := names.NewMachineTag(w.icfg.MachineId)
	_, err := w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
	}

	if w.icfg.Bootstrap != nil {
		if err = w.addLocalSnapUpload(); err != nil {
			return errors.Trace(err)
		}
		if err = w.addLocalControllerCharmsUpload(); err != nil {
			return errors.Trace(err)
		}
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

	w.conf.AddRunTextFile("/sbin/remove-juju-services", removeServicesScript, 0755)

	return w.addMachineAgentToBoot()
}

// runCmdToString converts a postruncmd or preruncmd value to a string.
// Per https://cloudinit.readthedocs.io/en/latest/topics/examples.html,
// these run commands can be either a string or a list of strings.
func runCmdToString(v any) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case []any: // beware! won't be be []string
		strs := make([]string, len(v))
		for i, sv := range v {
			ss, ok := sv.(string)
			if !ok {
				return "", errors.Errorf("expected list of strings, got list containing %T", sv)
			}
			strs[i] = ss
		}
		return utils.CommandString(strs...), nil
	default:
		return "", errors.Errorf("expected string or list of strings, got %T", v)
	}
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

func (w *unixConfigure) formatCurlProxyArguments() (proxyArgs string) {
	tools := w.icfg.ToolsList()[0]
	var proxySettings proxy.Settings
	if w.icfg.JujuProxySettings.HasProxySet() {
		proxySettings = w.icfg.JujuProxySettings
	} else if w.icfg.LegacyProxySettings.HasProxySet() {
		proxySettings = w.icfg.LegacyProxySettings
	}
	if strings.HasPrefix(tools.URL, httpSchemePrefix) && proxySettings.Http != "" {
		proxyUrl := proxySettings.Http
		proxyArgs += fmt.Sprintf(" --proxy %s", proxyUrl)
	} else if strings.HasPrefix(tools.URL, httpsSchemePrefix) && proxySettings.Https != "" {
		proxyUrl := proxySettings.Https
		// curl automatically uses HTTP CONNECT for URLs containing HTTPS
		proxyArgs += fmt.Sprintf(" --proxy %s", proxyUrl)
	}
	if proxySettings.NoProxy != "" {
		proxyArgs += fmt.Sprintf(" --noproxy %s", proxySettings.NoProxy)
	}
	return
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
	bootstrapParamsFile := path.Join(w.icfg.DataDir, FileNameBootstrapParams)
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

func (w *unixConfigure) addLocalSnapUpload() error {
	if w.icfg.Bootstrap == nil {
		return nil
	}

	snapPath := w.icfg.Bootstrap.JujuDbSnapPath
	assertionsPath := w.icfg.Bootstrap.JujuDbSnapAssertionsPath

	if snapPath == "" {
		return nil
	}

	logger.Infof("preparing to upload juju-db snap from %v", snapPath)
	snapData, err := stdos.ReadFile(snapPath)
	if err != nil {
		return errors.Trace(err)
	}
	_, snapName := path.Split(snapPath)
	w.conf.AddRunBinaryFile(path.Join(w.icfg.SnapDir(), snapName), snapData, 0644)

	logger.Infof("preparing to upload juju-db assertions from %v", assertionsPath)
	snapAssertionsData, err := stdos.ReadFile(assertionsPath)
	if err != nil {
		return errors.Trace(err)
	}
	_, snapAssertionsName := path.Split(assertionsPath)
	w.conf.AddRunBinaryFile(path.Join(w.icfg.SnapDir(), snapAssertionsName), snapAssertionsData, 0644)

	return nil
}

func (w *unixConfigure) addLocalControllerCharmsUpload() error {
	if w.icfg.Bootstrap == nil {
		return nil
	}

	charmPath := w.icfg.Bootstrap.ControllerCharm

	if charmPath == "" {
		return nil
	}

	logger.Infof("preparing to upload controller charm from %v", charmPath)
	_, err := charm.ReadCharm(charmPath)
	if err != nil {
		return errors.Trace(err)
	}
	var charmData []byte
	if charm.IsCharmDir(charmPath) {
		ch, err := charm.ReadCharmDir(charmPath)
		if err != nil {
			return errors.Trace(err)
		}
		buf := bytes.NewBuffer(nil)
		err = ch.ArchiveTo(buf)
		if err != nil {
			return errors.Trace(err)
		}
		charmData = buf.Bytes()
	} else {
		charmData, err = stdos.ReadFile(charmPath)
		if err != nil {
			return errors.Trace(err)
		}
	}
	w.conf.AddRunBinaryFile(path.Join(w.icfg.CharmDir(), bootstrap.ControllerCharmArchive), charmData, 0644)

	return nil
}

func (w *unixConfigure) addDownloadToolsCmds() error {
	tools := w.icfg.ToolsList()[0]
	if strings.HasPrefix(tools.URL, fileSchemePrefix) {
		toolsData, err := stdos.ReadFile(tools.URL[len(fileSchemePrefix):])
		if err != nil {
			return err
		}
		w.conf.AddRunBinaryFile(path.Join(w.icfg.JujuTools(), "tools.tar.gz"), toolsData, 0644)
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

			curlProxyArgs := w.formatCurlProxyArguments()
			curlCommand += curlProxyArgs
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
		fmt.Sprintf("echo -n %s > $bin/downloaded-tools.txt", shquote(string(toolsJson))),
	)

	return nil
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
