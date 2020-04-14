// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/winrm"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/manual"
)

// detectJujudProcess powershell script to determine
// if the jujud service is up and running in the machine
// if it's up the script will output "Yes" if it's down
// it will output "No"
const detectJujudProcess = `
	$jujuSvcs = Get-Service jujud-machine-*
	if($jujuSvcs -and $jujuSvcs[0].Status -eq "running"){
		return "Yes"
	}
	return "No"
`

// detectHardware is a powershell script that determines the following:
//  - the processor architecture
//		will try to determine the size of a int ptr, we know that any ptr on a x64 is
//		always 8 bytes and on x32 4 bytes always
//	- get the amount of ram the machine has
//		Use a WMI call to fetch the amount of RAM on the system. See:
//		https://msdn.microsoft.com/en-us/library/aa394347%28v=vs.85%29.aspx?f=255&MSPPError=-2147217396
//		for more details
//  - get the operating system name
//      compare the values we find in the registry with the version information
//		juju knows about. Once we find a match, we return the series
//  - get number of cores that the machine has
//		the process is using the Wmi windows Api to interrogate the os for
//		the physical number of cores that the machine has.
//
const detectHardware = `
function Get-Arch {
	$arch = (Get-ItemProperty "HKLM:\system\CurrentControlSet\Control\Session Manager\Environment").PROCESSOR_ARCHITECTURE
	return $arch.toString().ToLower()
}

function Get-Ram {
   # The comma is not a typo. It forces $capacity to be an array.
   # On machines with multiple slots of memory, the return value
   # of gcim win32_physicalmemory, may be an array. On a machine
   # with a single slot of memory, it will be a win32_physicalmemory
   # object. Forcing an array here, makes a common case out of
   # both situations and saves us the trouble of testing the
   # return type.
   $capacity = ,(gcim win32_physicalmemory).Capacity
   $ram = 0
   foreach($i in $capacity){
      $ram += $i
   }
   return $ram
}

function Get-OSName {
	$version = @{}
	{{ range $key, $value := . }}
$version.Add("{{$key}}", "{{$value}}")
	{{ end }}

	$v = $(Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion" -Name ProductName).ProductName
	$name = $v.Trim()

	# detection for nano server
	$k = $(Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Server\ServerLevels")
	$nano = $k.NanoServer
	if($nano -eq 1) {
		return $version["Windows Server 2016"].Trim()
	}

	foreach ($h in $version.keys) {
		$flag = $name.StartsWith($h)

		if ($flag) {
			return $version[$h].Trim()
		}
	}
}
function Get-NCores {
	# NumberOfProcessors will return the physical processor, but not the core count of each CPU. So if you have 2 quad core CPUs, NumberOfProcessors will return 2, not 8
    # Use NumberOfLogicalProcessors here (also takes into account hyperthreading).
    # see https://msdn.microsoft.com/en-us/library/aa394102%28v=vs.85%29.aspx?f=255&MSPPError=-2147217396 for more information
	Write-Host -NoNewline (gcim win32_computersystem).NumberOfLogicalProcessors
}
Get-Arch
Get-Ram
Get-OSName
Get-NCores
`

// newDetectHardwareScript will parse the detectHardware script and add
// into the powershell hastable the key,val of the map returned from the
// WindowsVersions func from the series pkg.
// it will return the script wrapped into a safe powershell base64
func newDetectHardwareScript() (string, error) {
	tmpl := template.Must(template.New("hc").Parse(detectHardware))
	var in bytes.Buffer
	seriesMap := series.WindowsVersions()
	if err := tmpl.Execute(&in, seriesMap); err != nil {
		return "", err
	}
	return shell.NewPSEncodedCommand(in.String())
}

// InitAdministratorUser will initially attempt to login as
// the Administrator user using the secure client
// only if this is false then this will make a new attempt with the unsecure http client.
func InitAdministratorUser(args *manual.ProvisionMachineArgs) error {
	logger.Infof("Trying https client as user %s on %s", args.Host, args.User)
	err := args.WinRM.Client.Ping()
	if err == nil {
		logger.Infof("Https connection is enabled on the host %s with user %s", args.Host, args.User)
		return nil
	}

	logger.Debugf("Https client authentication is not enabled on the host %s with user %s", args.Host, args.User)
	if args.WinRM.Client, err = winrm.NewClient(winrm.ClientConfig{
		User:     args.User,
		Host:     args.Host,
		Timeout:  25 * time.Second,
		Password: winrm.TTYGetPasswd,
		Secure:   false,
	}); err != nil {
		return errors.Annotatef(err, "cannot create a new http winrm client ")
	}

	logger.Infof("Trying http client as user %s on %s", args.Host, args.User)
	if err = args.WinRM.Client.Ping(); err != nil {
		logger.Debugf("WinRM insecure listener is not enabled on %s", args.Host)
		return errors.Annotatef(err, "cannot provision, because all winrm default connections failed")
	}

	defClient := args.WinRM.Client
	logger.Infof("Trying to enable https client certificate authentication")
	if args.WinRM.Client, err = enableCertAuth(args); err != nil {
		logger.Infof("Cannot enable client auth cert authentication for winrm")
		logger.Infof("Reverting back to insecure client interaction")
		args.WinRM.Client = defClient
		return nil
	}

	logger.Infof("Client certs are installed and setup on the %s with user %s", args.Host, args.User)
	err = args.WinRM.Client.Ping()
	if err == nil {
		return nil
	}

	logger.Infof("Winrm https connection is broken, cannot retrieve a response")
	logger.Infof("Reverting back to insecure client interactions")
	args.WinRM.Client = defClient

	return nil

}

// enableCertAuth enables https cert auth interactions
// with the winrm listener and returns the client
func enableCertAuth(args *manual.ProvisionMachineArgs) (manual.WinrmClientAPI, error) {
	var stderr bytes.Buffer
	pass := args.WinRM.Client.Password()

	scripts, err := bindInitScripts(pass, args.WinRM.Keys)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, script := range scripts {
		err = args.WinRM.Client.Run(script, args.Stdout, &stderr)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	cfg := winrm.ClientConfig{
		User:    args.User,
		Host:    args.Host,
		Key:     args.WinRM.Keys.ClientKey(),
		Cert:    args.WinRM.Keys.ClientCert(),
		Timeout: 25 * time.Second,
		Secure:  true,
	}

	caCert := args.WinRM.Keys.CACert()
	if caCert == nil {
		logger.Infof("Skipping winrm CA validation")
		cfg.Insecure = true
	} else {
		cfg.CACert = caCert
	}

	return winrm.NewClient(cfg)
}

// bindInitScripts creates a series of scripts in a standard
// (utf-16-le, base64)format for passing to the winrm conn to be executed remotely
// we are doing this instead of one big script because winrm supports
// just 8192 length commands. We know we have an amount of prefixed scripts
// that we want to bind for the init process so create an array of scripts
func bindInitScripts(pass string, keys *winrm.X509) ([]string, error) {
	var (
		err error
	)

	scripts := make([]string, 3, 3)

	if len(pass) == 0 {
		return scripts, fmt.Errorf("The password is empty, provide a valid password to enable https interactions")
	}

	scripts[0], err = shell.NewPSEncodedCommand(setFiles)
	if err != nil {
		return nil, err
	}

	scripts[1] = fmt.Sprintf(setFilesContent, string(keys.ClientCert()))

	scripts[1], err = shell.NewPSEncodedCommand(scripts[1])
	if err != nil {
		return nil, err
	}

	scripts[2] = fmt.Sprintf(setConnWinrm, pass)
	scripts[2], err = shell.NewPSEncodedCommand(scripts[2])
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// setFiles powershell script that will manage and create the conf folder and files
const setFiles = `
$jujuHome = [io.path]::Combine($ENV:APPDATA, 'Juju')
$x509Path = [io.path]::Combine($jujuHome, 'x509')
$certPath = [io.path]::Combine($x509Path,'winrmcert.crt')
if (-Not (Test-Path $jujuHome)) {
	New-Item $jujuHome -Type directory
}
if (-Not (Test-Path $x509Path)) {
	New-Item $x509Path -Type directory
}
if (-Not (Test-Path $certPath)) {
	New-Item $certPath -Type file
}
`

// setFilesContent powershell script that will write
// x509 cert and key into the juju conf.
const setFilesContent = `
$cert=@"
%s
"@
[io.file]::WriteAllText("$ENV:APPDATA\Juju\x509\winrmcert.crt", $cert)
`

// setConnWinrm powershell script that will create and write the client cert from the juju conf file into the windows
// target enabling winrm secure client interactions with the machine
const setConnWinrm = `
winrm set winrm/config/winrs '@{MaxMemoryPerShellMB="1024"}'
winrm set winrm/config/client/auth '@{Digest="false"}'
winrm set winrm/config/service/auth '@{Certificate="true"}'
Remove-Item -Path WSMan:\localhost\ClientCertificate\ClientCertificate_* -Recurse -force | Out-null
$username = "Administrator"
$password = "%s"
$client_cert_path = [io.path]::Combine($env:APPDATA, 'Juju', 'x509', 'winrmcert.crt')
$clientcert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($client_cert_path)
$castore = New-Object System.Security.Cryptography.X509Certificates.X509Store(
	    [System.Security.Cryptography.X509Certificates.StoreName]::Root,
		[System.Security.Cryptography.X509Certificates.StoreLocation]::LocalMachine)
$castore.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
$castore.Add($clientcert)
$subject = [string]::Join([CHAR][BYTE]32, "juju", "winrm", "client", "cert")
$secure_password = ConvertTo-SecureString $password -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential "$ENV:COMPUTERNAME\$username", $secure_password
New-Item -Path WSMan:\localhost\ClientCertificate -Issuer $clientcert.Thumbprint -Subject $subject -Uri * -Credential $cred -Force
`

// gatherMachineParams collects all the information we know about the machine
// we are about to provision. It will winrm into that machine as the provision user
// The hostname supplied should not include a username.
// If we can, we will reverse lookup the hostname by its IP address, and use
// the DNS resolved name, rather than the name that was supplied
func gatherMachineParams(hostname string, cli manual.WinrmClientAPI) (*params.AddMachineParams, error) {
	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	addr, err := manual.HostAddress(hostname)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to compute public address for %q", hostname)
	}

	provisioned, err := checkProvisioned(hostname, cli)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot decide if the machine is provisioned")
	}
	if provisioned {
		return nil, manual.ErrProvisioned
	}

	hc, ser, err := DetectSeriesAndHardwareCharacteristics(hostname, cli)
	if err != nil {
		err = fmt.Errorf("error detecting windows characteristics: %v", err)
		return nil, err
	}
	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.

	instanceId := instance.Id(manual.ManualInstancePrefix + hostname)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid)
	machineParams := &params.AddMachineParams{
		Series:                  ser,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   params.FromProviderAddresses(addr),
		Jobs:                    []model.MachineJob{model.JobHostUnits},
	}
	return machineParams, nil
}

// checkProvisioned checks if the machine is already provisioned or not.
// if it's already provisioned it will return true
func checkProvisioned(host string, cli manual.WinrmClientAPI) (bool, error) {
	logger.Tracef("Checking if %s windows machine is already provisioned", host)
	var stdout, stderr bytes.Buffer

	// run the command trough winrm
	// this script detects if the jujud process, service is up and running
	script, err := shell.NewPSEncodedCommand(detectJujudProcess)
	if err != nil {
		return false, errors.Trace(err)
	}

	// send the script to the windows machine
	if err = cli.Run(script, &stdout, &stderr); err != nil {
		return false, errors.Trace(err)
	}

	provisioned := strings.Contains(stdout.String(), "Yes")
	// if the script said yes
	if provisioned {
		logger.Infof("%s is already provisioned", host)
	} else {
		logger.Infof("%s is not provisioned", host)
	}

	return provisioned, err
}

// DetectSeriesAndHardwareCharacteristics detects the windows OS
// series and hardware characteristics of the remote machine
// by connecting to the machine and executing a bash script.
func DetectSeriesAndHardwareCharacteristics(host string, cli manual.WinrmClientAPI) (hc instance.HardwareCharacteristics, series string, err error) {
	logger.Infof("Detecting series and characteristics on %s windows machine", host)
	var stdout, stderr bytes.Buffer

	script, err := newDetectHardwareScript()
	if err != nil {
		return hc, "", err
	}

	// send the script to the windows machine
	if err = cli.Run(script, &stdout, &stderr); err != nil {
		return hc, "", errors.Trace(err)
	}

	info, err := splitHardWareScript(stdout.String())
	if err != nil {
		return hc, "", errors.Trace(err)
	}

	series = strings.Replace(info[2], "\r", "", -1)

	if err = initHC(&hc, info); err != nil {
		return hc, "", errors.Trace(err)
	}

	return hc, series, nil
}

// initHC it will initialize the hardware characteristics struct with the
// parsed and checked info slice string
// info description :
//  - info[0] the arch of the machine
//  - info[1] the amount of memory that the machine has
//  - info[2] the series of the machine
//  - info[3] the number of cores that the machine has
// It returns nil if it parsed successfully.
func initHC(hc *instance.HardwareCharacteristics, info []string) error {
	// add arch
	arch := arch.NormaliseArch(info[0])
	hc.Arch = &arch

	// parse the mem number
	mem, err := strconv.ParseUint(info[1], 10, 64)
	if err != nil {
		return errors.Annotatef(err, "Can't parse mem number of the windows machine")
	}
	hc.Mem = new(uint64)
	*hc.Mem = mem

	// parse the core number
	cores, err := strconv.ParseUint(info[3], 10, 64)
	if err != nil {
		return errors.Annotatef(err, "Can't parse cores number of the windows machine")
	}

	hc.CpuCores = new(uint64)
	*hc.CpuCores = cores
	return nil
}

// splitHardwareScript will split the result from the detectHardware powershell script
// to extract the information in a specific order.
// this will return a slice of string that will be used in conjunctions with the above function
func splitHardWareScript(script string) ([]string, error) {
	scr := strings.Split(script, "\n")
	n := len(scr)
	if n < 3 {
		return nil, fmt.Errorf("No hardware fields on running the powershell deteciton script, %s", script)
	}
	for i := 0; i < n; i++ {
		scr[i] = strings.TrimSpace(scr[i])
	}
	return scr, nil
}

// RunProvisionScript exported for testing purposes
var RunProvisionScript = runProvisionScript

// runProvisionScript runs the script generated by the provisioner
// the script can be big and the underlying protocol dosen't support long messages
// we need to send it into little chunks saving first into a file and then execute it.
func runProvisionScript(script string, cli manual.WinrmClientAPI, stdin, stderr io.Writer) (err error) {
	script64 := base64.StdEncoding.EncodeToString([]byte(script))
	input := bytes.NewBufferString(script64) // make new buffer out of script
	// we must make sure to buffer the entire script
	// in a sequential write fashion first into a script
	// decouple the provisioning script into little 1024 byte chunks
	// we are doing this in order to append into a .ps1 file.
	var buf [1024]byte

	// if the file dosen't exist ,create it
	// if the file exists just clear/reset it
	script, err = shell.NewPSEncodedCommand(initChunk)
	if err != nil {
		return err
	}
	if err = cli.Run(script, stdin, stderr); err != nil {
		return errors.Trace(err)
	}

	// sequential read.
	for input.Len() != 0 {
		n, err := input.Read(buf[:])
		if err != nil && err != io.EOF {
			return errors.Trace(err)
		}
		script, err = shell.NewPSEncodedCommand(
			fmt.Sprintf(saveChunk, string(buf[:n])),
		)
		if err != nil {
			return errors.Trace(err)
		}
		if err = cli.Run(script, stdin, stderr); err != nil {
			return errors.Trace(err)
		}
	}

	// after the sendAndSave script is successfully done
	// we must execute the newly writed script
	script, err = shell.NewPSEncodedCommand(runCmdProv)
	if err != nil {
		return err
	}
	logger.Debugf("Running the provisioningScript")
	var outerr bytes.Buffer
	if err = cli.Run(script, stdin, &outerr); err != nil {
		return errors.Trace(err)
	}

	return err
}

// initChunk creates or clears the file that the userdata will be appendend.
const initChunk = `
$provisioningDir = [io.path]::Combine($ENV:APPDATA, 'Juju')
if (-not (Test-Path $provisioningDir)){
    mkdir $provisioningDir | Out-Null
}
$provisionPath = [io.path]::Combine($provisioningDir, 'provision.ps1')
if (-Not (Test-Path $provisionPath)) {
	New-Item $provisionPath -Type file
} else {
	Clear-Content $provisionPath
}

`

// saveChunk powershell script that will append into the userdata file created after the
// initChunk executed.
// this will be called multiple times in a sequential order.
const saveChunk = `
$chunk= @"
%s
"@
$provisionPath= [io.path]::Combine($ENV:APPDATA, 'Juju', 'provision.ps1')
$stream = New-Object System.IO.StreamWriter -ArgumentList ([IO.File]::Open($provisionPath, "Append"))
$stream.Write($chunk)
$stream.close()
`

// runCmdProv powrshell script that decodes and executes the newly created userdata file
// after the process of writing the sequantial script is done above
const runCmdProv = `
$provisionPath= [io.path]::Combine($ENV:APPDATA, 'Juju', 'provision.ps1')
$script = [IO.File]::ReadAllText($provisionPath)
$x = [System.Text.Encoding]::ASCII.GetString([System.Convert]::FromBase64String($script))
Set-Content C:\udata.ps1 $x
powershell.exe -ExecutionPolicy RemoteSigned -NonInteractive -File C:\udata.ps1
`

// ProvisioningScript generates a powershell script that can be
// executed on a remote host to carry out the cloud-init
// configuration.
func ProvisioningScript(icfg *instancecfg.InstanceConfig) (string, error) {
	cloudcfg, err := cloudinit.New(icfg.Series)
	if err != nil {
		return "", errors.Annotate(err, "error creating new cloud config")
	}

	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	if err != nil {
		return "", errors.Annotate(err, "error creating new userdata based on the cloud config")
	}

	if err := udata.Configure(); err != nil {
		return "", errors.Annotate(err, "error adding extra configurations in the userdata")
	}

	return cloudcfg.RenderScript()
}
