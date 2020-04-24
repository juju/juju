// Copyright 2012, 2013, 2014, 2015 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	"github.com/juju/utils/cert"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/tools"
)

//go:generate go run github.com/juju/juju/generate/filetoconst UserDataScript windowsuserdatafiles/userdata.ps1 winuserdatawrapper.go 2016 cloudconfig
//go:generate go run github.com/juju/juju/generate/winuserdata 2016 winuserdata.go cloudconfig

type aclType string

const (
	fileSystem    aclType = "FileSystem"
	registryEntry aclType = "Registry"
)

type windowsConfigure struct {
	baseConfigure
}

// Configure updates the provided cloudinit.Config with
// configuration to initialize a Juju machine agent.
func (w *windowsConfigure) Configure() error {
	if err := w.ConfigureBasic(); err != nil {
		return err
	}
	if err := w.ConfigureJuju(); err != nil {
		return err
	}
	return w.ConfigureCustomOverrides()
}

// ConfigureBasic implements UserdataConfig.ConfigureBasic
func (w *windowsConfigure) ConfigureBasic() error {

	tmpDir, err := paths.TempDir(w.icfg.Series)
	if err != nil {
		return err
	}

	renderer := w.conf.ShellRenderer()
	dataDir := renderer.FromSlash(w.icfg.DataDir)
	transientDataDir := renderer.FromSlash(w.icfg.TransientDataDir)
	baseDir := renderer.FromSlash(filepath.Dir(tmpDir))
	binDir := renderer.Join(baseDir, "bin")

	w.conf.AddScripts(windowsPowershellHelpers)

	// The jujud user only gets created on non-nano versions for now.
	if !series.IsWindowsNano(w.icfg.Series) {
		w.conf.AddScripts(addJujuUser)
	}

	w.conf.AddScripts(
		// Some providers create a baseDir before this step, but we need to
		// make sure it exists before applying icacls
		fmt.Sprintf(`mkdir -Force "%s"`, renderer.FromSlash(baseDir)),
		fmt.Sprintf(`mkdir %s`, renderer.FromSlash(tmpDir)),
		fmt.Sprintf(`mkdir "%s"`, binDir),
		fmt.Sprintf(`mkdir "%s"`, transientDataDir),
		fmt.Sprintf(`mkdir "%s\locks"`, renderer.FromSlash(dataDir)),
		`setx /m PATH "$env:PATH;C:\Juju\bin\"`,
		// This is necessary for setACLs to work
		`$adminsGroup = (New-Object System.Security.Principal.SecurityIdentifier("S-1-5-32-544")).Translate([System.Security.Principal.NTAccount])`,
		fmt.Sprintf(`icacls "%s" /inheritance:r /grant "${adminsGroup}:(OI)(CI)(F)" /t`, renderer.FromSlash(baseDir)),
	)

	// TODO(bogdanteleaga): This, together with the call above, should be using setACLs, once it starts working across all windows versions properly.
	// Until then, if we change permissions, both this and setACLs should be changed to do the same thing.
	if !series.IsWindowsNano(w.icfg.Series) {
		w.conf.AddScripts(fmt.Sprintf(`icacls "%s" /inheritance:r /grant "jujud:(OI)(CI)(F)" /t`, renderer.FromSlash(baseDir)))
	}

	noncefile := renderer.Join(dataDir, NonceFile)
	w.conf.AddScripts(
		fmt.Sprintf(`Set-Content "%s" "%s"`, noncefile, shquote(w.icfg.MachineNonce)),
	)
	return nil
}

// ConfigureJuju implements UserdataConfig.ConfigureJuju
func (w *windowsConfigure) ConfigureJuju() error {
	if err := w.icfg.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if w.icfg.Controller != nil {
		return errors.Errorf("controllers not supported on windows")
	}

	tools := w.icfg.ToolsList()[0]
	toolsJson, err := json.Marshal(tools)
	if err != nil {
		return errors.Annotate(err, "while serializing the agent binaries")
	}

	renderer := w.conf.ShellRenderer()
	w.conf.AddScripts(
		fmt.Sprintf(`$binDir="%s"`, renderer.FromSlash(w.icfg.JujuTools())),
		fmt.Sprintf(`mkdir '%s'`, renderer.FromSlash(w.icfg.LogDir)),
		`mkdir $binDir`,
	)

	toolsDownloadCmds, err := addDownloadToolsCmds(
		w.icfg.Series, w.icfg.APIInfo.CACert, w.icfg.ToolsList(),
	)
	if err != nil {
		return errors.Trace(err)
	}
	w.conf.AddScripts(toolsDownloadCmds...)

	w.conf.AddScripts(
		`$dToolsHash = Get-FileSHA256 -FilePath "$binDir\tools.tar.gz"`,
		fmt.Sprintf(`$dToolsHash > "$binDir\juju%s.sha256"`, tools.Version),
		fmt.Sprintf(`if ($dToolsHash.ToLower() -ne "%s"){ Throw "Tools checksum mismatch"}`,
			tools.SHA256),
		fmt.Sprintf(`GUnZip-File -infile $binDir\tools.tar.gz -outdir $binDir`),
		`rm "$binDir\tools.tar*"`,
		fmt.Sprintf(`Set-Content $binDir\downloaded-tools.txt '%s'`, string(toolsJson)),
	)

	for _, cmd := range createJujuRegistryKeyCmds(w.icfg.Series) {
		w.conf.AddRunCmd(cmd)
	}

	machineTag := names.NewMachineTag(w.icfg.MachineId)
	_, err = w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	return w.addMachineAgentToBoot()
}

// ConfigureCustomOverrides implements UserdataConfig.ConfigureCustomOverrides
func (w *windowsConfigure) ConfigureCustomOverrides() error {
	// TODO HML 2017-12-08
	// Implement for Windows support of model-config cloudinit-userdata.
	return nil
}

// createJujuRegistryKeyCmds is going to create a juju registry key and set
// permissions on it such that it's only accessible to administrators
func createJujuRegistryKeyCmds(series string) []string {
	aclCmds := setACLs(osenv.JujuRegistryKey, registryEntry, series)
	regCmds := []string{

		// Create a registry key for storing juju related information
		fmt.Sprintf(`New-Item -Path '%s'`, osenv.JujuRegistryKey),

		// Create a JUJU_DEV_FEATURE_FLAGS entry which may or may not be empty.
		fmt.Sprintf(`New-ItemProperty -Path '%s' -Name '%s'`,
			osenv.JujuRegistryKey,
			osenv.JujuFeatureFlagEnvKey),
		fmt.Sprintf(`Set-ItemProperty -Path '%s' -Name '%s' -Value '%s'`,
			osenv.JujuRegistryKey,
			osenv.JujuFeatureFlagEnvKey,
			featureflag.AsEnvironmentValue()),
	}
	return append(regCmds[:1], append(aclCmds, regCmds[1:]...)...)
}

func setACLs(path string, permType aclType, ser string) []string {
	ruleModel := `$rule = New-Object System.Security.AccessControl.%sAccessRule %s`
	permModel := `%s = "%s", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"`
	adminPermVar := `$adminPerm`
	jujudPermVar := `$jujudPerm`

	rulesToAdd := []string{
		// $adminsGroup must be defined before calling setACLs
		fmt.Sprintf(permModel, adminPermVar, `$adminsGroup`),
		fmt.Sprintf(ruleModel, permType, adminPermVar),
		`$acl.AddAccessRule($rule)`,
	}

	if !series.IsWindowsNano(ser) {
		jujudUserACLRules := []string{
			fmt.Sprintf(permModel, jujudPermVar, `jujud`),
			fmt.Sprintf(ruleModel, permType, jujudPermVar),
			`$acl.AddAccessRule($rule)`,
		}

		rulesToAdd = append(rulesToAdd, jujudUserACLRules...)
	}

	aclCmds := []string{
		fmt.Sprintf(`$acl = Get-Acl -Path '%s'`, path),

		// Reset the ACL's on it and add administrator access only.
		`$acl.SetAccessRuleProtection($true, $false)`,

		fmt.Sprintf(`Set-Acl -Path '%s' -AclObject $acl`, path),
	}

	return append(aclCmds[:2], append(rulesToAdd, aclCmds[2:]...)...)
}

func addDownloadToolsCmds(ser string, certificate string, toolsList tools.List) ([]string, error) {
	var cmds []string
	var getDownloadFileCmd func(url string) string
	if series.IsWindowsNano(ser) {
		parsedCert, err := cert.ParseCert(certificate)
		if err != nil {
			return nil, err
		}
		caCert := base64.StdEncoding.EncodeToString(parsedCert.Raw)
		cmds = []string{fmt.Sprintf(`$cacert = "%s"`, caCert),
			`$cert_bytes = $cacert | %{ ,[System.Text.Encoding]::UTF8.GetBytes($_) }`,
			`$cert = new-object System.Security.Cryptography.X509Certificates.X509Certificate2(,$cert_bytes)`,
			`$store = Get-Item Cert:\LocalMachine\AuthRoot`,
			`$store.Open("ReadWrite")`,
			`$store.Add($cert)`,
		}
		getDownloadFileCmd = func(url string) string {
			return fmt.Sprintf(`Invoke-FastWebRequest -URI '%s' -OutFile "$binDir\tools.tar.gz"`, url)
		}
	} else {
		cmds = []string{
			`$WebClient = New-Object System.Net.WebClient`,
			`[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}`,
			`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12`,
		}
		getDownloadFileCmd = func(url string) string {
			return fmt.Sprintf(`$WebClient.DownloadFile('%s', "$binDir\tools.tar.gz");`, url)
		}
	}

	// Attempt all of the URLs, one after the other, until one succeeds.
	// If all of the URLs fail, we retry the whole lot. We retry in this
	// way, rather than retrying individually, to avoid one permanently
	// bad URL from holding up the download.
	downloadCmds := make([]string, len(toolsList))
	for i, tools := range toolsList {
		downloadCmds[i] = fmt.Sprintf("{ %s }", getDownloadFileCmd(tools.URL))
	}
	downloadCmd := fmt.Sprintf("ExecRetry { TryExecAll @(%s) }", strings.Join(downloadCmds, ", "))
	cmds = append(cmds, downloadCmd)

	return cmds, nil
}
