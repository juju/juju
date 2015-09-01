// Copyright 2012, 2013, 2014, 2015 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/paths"
)

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
	return w.ConfigureJuju()
}

func (w *windowsConfigure) ConfigureBasic() error {

	series := w.icfg.Series
	tmpDir, err := paths.TempDir(series)
	if err != nil {
		return err
	}
	renderer := w.conf.ShellRenderer()
	dataDir := renderer.FromSlash(w.icfg.DataDir)
	baseDir := renderer.FromSlash(filepath.Dir(tmpDir))
	binDir := renderer.Join(baseDir, "bin")

	w.conf.AddScripts(
		fmt.Sprintf(`%s`, winPowershellHelperFunctions),

		// Some providers create a baseDir before this step, but we need to
		// make sure it exists before applying icacls
		fmt.Sprintf(`mkdir -Force "%s"`, renderer.FromSlash(baseDir)),
		fmt.Sprintf(`mkdir %s`, renderer.FromSlash(tmpDir)),
		fmt.Sprintf(`mkdir "%s"`, binDir),
		fmt.Sprintf(`mkdir "%s\locks"`, renderer.FromSlash(dataDir)),
	)

	// This is necessary for setACLs to work
	w.conf.AddScripts(`$adminsGroup = (New-Object System.Security.Principal.SecurityIdentifier("S-1-5-32-544")).Translate([System.Security.Principal.NTAccount])`)
	w.conf.AddScripts(setACLs(renderer.FromSlash(baseDir), fileSystem)...)
	w.conf.AddScripts(`setx /m PATH "$env:PATH;C:\Juju\bin\"`)
	noncefile := renderer.Join(dataDir, NonceFile)
	w.conf.AddScripts(
		fmt.Sprintf(`Set-Content "%s" "%s"`, noncefile, shquote(w.icfg.MachineNonce)),
	)
	return nil
}

func (w *windowsConfigure) ConfigureJuju() error {
	if err := w.icfg.VerifyConfig(); err != nil {
		return errors.Trace(err)
	}
	if w.icfg.Bootstrap == true {
		// Bootstrap machine not supported on windows
		return errors.Errorf("bootstrapping is not supported on windows")
	}

	toolsJson, err := json.Marshal(w.icfg.Tools)
	if err != nil {
		return errors.Annotate(err, "while serializing the tools")
	}
	renderer := w.conf.ShellRenderer()
	w.conf.AddScripts(
		fmt.Sprintf(`$binDir="%s"`, renderer.FromSlash(w.icfg.JujuTools())),
		fmt.Sprintf(`mkdir '%s'`, renderer.FromSlash(w.icfg.LogDir)),
		`mkdir $binDir`,
		`$WebClient = New-Object System.Net.WebClient`,
		`[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}`,
		fmt.Sprintf(`ExecRetry { $WebClient.DownloadFile('%s', "$binDir\tools.tar.gz") }`, w.icfg.Tools.URL),
		`$dToolsHash = Get-FileSHA256 -FilePath "$binDir\tools.tar.gz"`,
		fmt.Sprintf(`$dToolsHash > "$binDir\juju%s.sha256"`,
			w.icfg.Tools.Version),
		fmt.Sprintf(`if ($dToolsHash.ToLower() -ne "%s"){ Throw "Tools checksum mismatch"}`,
			w.icfg.Tools.SHA256),
		fmt.Sprintf(`GUnZip-File -infile $binDir\tools.tar.gz -outdir $binDir`),
		`rm "$binDir\tools.tar*"`,
		fmt.Sprintf(`Set-Content $binDir\downloaded-tools.txt '%s'`, string(toolsJson)),
	)

	for _, cmd := range CreateJujuRegistryKeyCmds() {
		w.conf.AddRunCmd(cmd)
	}

	machineTag := names.NewMachineTag(w.icfg.MachineId)
	_, err = w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	return w.addMachineAgentToBoot()
}

// CreateJujuRegistryKey is going to create a juju registry key and set
// permissions on it such that it's only accessible to administrators
// It is exported because it is used in an upgrade step
func CreateJujuRegistryKeyCmds() []string {
	aclCmds := setACLs(osenv.JujuRegistryKey, registryEntry)
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

func setACLs(path string, permType aclType) []string {
	ruleModel := `$rule = New-Object System.Security.AccessControl.%sAccessRule %s`
	permModel := `%s = "%s", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"`
	adminPermVar := `$adminPerm`
	jujudPermVar := `$jujudPerm`
	return []string{
		fmt.Sprintf(`$acl = Get-Acl -Path '%s'`, path),

		// Reset the ACL's on it and add administrator access only.
		`$acl.SetAccessRuleProtection($true, $false)`,

		// $adminsGroup must be defined before calling setACLs
		fmt.Sprintf(permModel, adminPermVar, `$adminsGroup`),
		fmt.Sprintf(permModel, jujudPermVar, `jujud`),
		fmt.Sprintf(ruleModel, permType, adminPermVar),
		`$acl.AddAccessRule($rule)`,
		fmt.Sprintf(ruleModel, permType, jujudPermVar),
		`$acl.AddAccessRule($rule)`,
		fmt.Sprintf(`Set-Acl -Path '%s' -AclObject $acl`, path),
	}
}
