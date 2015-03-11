// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/paths"
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

	series := w.mcfg.Series
	tmpDir, err := paths.TempDir(series)
	if err != nil {
		return err
	}
	renderer := w.conf.ShellRenderer
	dataDir := renderer.FromSlash(w.mcfg.DataDir)
	baseDir := renderer.FromSlash(filepath.Dir(tmpDir))
	binDir := renderer.Join(baseDir, "bin")

	w.conf.AddScripts(
		fmt.Sprintf(`%s`, winPowershellHelperFunctions),
		fmt.Sprintf(`icacls "%s" /grant "jujud:(OI)(CI)(F)" /T`, renderer.FromSlash(baseDir)),
		fmt.Sprintf(`mkdir %s`, renderer.FromSlash(tmpDir)),
		fmt.Sprintf(`mkdir "%s"`, binDir),
		fmt.Sprintf(`%s`, winSetPasswdScript),
		fmt.Sprintf(`Start-ProcessAsUser -Command $powershell -Arguments "-File C:\juju\bin\save_pass.ps1 $juju_passwd" -Credential $jujuCreds`),
		fmt.Sprintf(`mkdir "%s\locks"`, renderer.FromSlash(dataDir)),
		fmt.Sprintf(`Start-ProcessAsUser -Command $cmdExe -Arguments '/C setx PATH "%%PATH%%;C:\Juju\bin"' -Credential $jujuCreds`),
	)
	noncefile := renderer.Join(dataDir, NonceFile)
	w.conf.AddScripts(
		fmt.Sprintf(`Set-Content "%s" "%s"`, noncefile, shquote(w.mcfg.MachineNonce)),
	)
	return nil
}

func (w *windowsConfigure) ConfigureJuju() error {
	if err := verifyConfig(w.mcfg); err != nil {
		return errors.Annotate(err, "while verifying machine config")
	}
	toolsJson, err := json.Marshal(w.mcfg.Tools)
	if err != nil {
		return errors.Annotate(err, "while serializing the tools")
	}
	const python = `${env:ProgramFiles(x86)}\Cloudbase Solutions\Cloudbase-Init\Python27\python.exe`
	renderer := w.conf.ShellRenderer
	w.conf.AddScripts(
		fmt.Sprintf(`$binDir="%s"`, renderer.FromSlash(w.mcfg.jujuTools())),
		`$tmpBinDir=$binDir.Replace('\', '\\')`,
		fmt.Sprintf(`mkdir '%s'`, renderer.FromSlash(w.mcfg.LogDir)),
		`mkdir $binDir`,
		`$WebClient = New-Object System.Net.WebClient`,
		`[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}`,
		fmt.Sprintf(`ExecRetry { $WebClient.DownloadFile('%s', "$binDir\tools.tar.gz") }`, w.mcfg.Tools.URL),
		`$dToolsHash = (Get-FileHash -Algorithm SHA256 "$binDir\tools.tar.gz").hash`,
		fmt.Sprintf(`$dToolsHash > "$binDir\juju%s.sha256"`,
			w.mcfg.Tools.Version),
		fmt.Sprintf(`if ($dToolsHash.ToLower() -ne "%s"){ Throw "Tools checksum mismatch"}`,
			w.mcfg.Tools.SHA256),
		fmt.Sprintf(`& "%s" -c "import tarfile;archive = tarfile.open('$tmpBinDir\\tools.tar.gz');archive.extractall(path='$tmpBinDir')"`, python),
		`rm "$binDir\tools.tar*"`,
		fmt.Sprintf(`Set-Content $binDir\downloaded-tools.txt '%s'`, string(toolsJson)),
	)

	if w.mcfg.Bootstrap == true {
		// Bootstrap machine not supported on windows
		return errors.Errorf("Bootstrap node is not supported on Windows.")
	}

	machineTag := names.NewMachineTag(w.mcfg.MachineId)
	_, err = w.addAgentInfo(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	return w.addMachineAgentToBoot()
}
