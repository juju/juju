function create-account ([string]$accountName, [string]$accountDescription, [string]$password) {
	$hostname = hostname
	$comp = [adsi]"WinNT://$hostname"
	$user = $comp.Create("User", $accountName)
	$user.SetPassword($password)
	$user.SetInfo()
	$user.description = $accountDescription
	$user.SetInfo()
	$User.UserFlags[0] = $User.UserFlags[0] -bor 0x10000
	$user.SetInfo()

	# This gets the Administrator group name that is localized on different windows versions. 
	# However the SID S-1-5-32-544 is the same on all versions.
	$adminGroup = (New-Object System.Security.Principal.SecurityIdentifier("S-1-5-32-544")).Translate([System.Security.Principal.NTAccount]).Value.Split("\")[1]

	$objOU = [ADSI]"WinNT://$hostname/$adminGroup,group"
	$objOU.add("WinNT://$hostname/$accountName")
}

$Source = @"
%s
"@

Add-Type -TypeDefinition $Source -Language CSharp

function Get-RandomPassword
{
	[CmdletBinding()]
	param
	(
		[parameter(Mandatory=$true)]
		[int]$Length
	)
	process
	{
		$hProvider = 0
		try
		{
			if(![PSCloudbase.Win32CryptApi]::CryptAcquireContext([ref]$hProvider, $null, $null,
																 [PSCloudbase.Win32CryptApi]::PROV_RSA_FULL,
																 ([PSCloudbase.Win32CryptApi]::CRYPT_VERIFYCONTEXT -bor
																  [PSCloudbase.Win32CryptApi]::CRYPT_SILENT)))
			{
				throw "CryptAcquireContext failed with error: 0x" + "{0:X0}" -f [PSCloudbase.Win32CryptApi]::GetLastError()
			}

			$buffer = New-Object byte[] $Length
			if(![PSCloudbase.Win32CryptApi]::CryptGenRandom($hProvider, $Length, $buffer))
			{
				throw "CryptGenRandom failed with error: 0x" + "{0:X0}" -f [PSCloudbase.Win32CryptApi]::GetLastError()
			}

			$buffer | ForEach-Object { $password += "{0:X0}" -f $_ }
			return $password
		}
		finally
		{
			if($hProvider)
			{
				$retVal = [PSCloudbase.Win32CryptApi]::CryptReleaseContext($hProvider, 0)
			}
		}
	}
}

$SourcePolicy = @"
%s
"@

Add-Type -TypeDefinition $SourcePolicy -Language CSharp

function SetAssignPrimaryTokenPrivilege($UserName)
{
	$privilege = "SeAssignPrimaryTokenPrivilege"
	if (!([PSCarbon.Lsa]::GetPrivileges($UserName) -contains $privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

function SetUserLogonAsServiceRights($UserName)
{
	$privilege = "SeServiceLogonRight"
	if (!([PSCarbon.Lsa]::GetPrivileges($UserName) -Contains $privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

$juju_passwd = Get-RandomPassword 20
$juju_passwd += "^"
create-account jujud "Juju Admin user" $juju_passwd
$hostname = hostname
$juju_user = "$hostname\jujud"

SetUserLogonAsServiceRights $juju_user
SetAssignPrimaryTokenPrivilege $juju_user

$path = "HKLM:\Software\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList"
if(!(Test-Path $path)){
	New-Item -Path $path -force
}
New-ItemProperty $path -Name "jujud" -Value 0 -PropertyType "DWord"

$secpasswd = ConvertTo-SecureString $juju_passwd -AsPlainText -Force
$jujuCreds = New-Object System.Management.Automation.PSCredential ($juju_user, $secpasswd)
