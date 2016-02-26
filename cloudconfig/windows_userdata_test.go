// Copyright 2014, 2015 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions
// Copyright 2012 Aaron Jensen
//
// Licensed under the AGPLv3, see LICENCE file for details.
//
// This file borrowed some code from https://bitbucket.org/splatteredbits/carbon
// (see Source/Security/Privilege.cs). This external source is licensed under
// Apache-2.0 license which is compatible with AGPLv3 license. Because it's
// compatible we can and have licensed this derived work under AGPLv3. The original
// Apache-2.0 license for the external source can be found inside Apache-License.txt.
// Copyright statement of the external source: Copyright 2012 Aaron Jensen

package cloudconfig_test

var WindowsUserdata = `#ps1_sysnative



$ErrorActionPreference = "Stop"

function ExecRetry($command, $retryInterval = 15)
{
	$currErrorActionPreference = $ErrorActionPreference
	$ErrorActionPreference = "Continue"

	while ($true)
	{
		try
		{
			& $command
			break
		}
		catch [System.Exception]
		{
			Write-Error $_.Exception
			Start-Sleep $retryInterval
		}
	}

	$ErrorActionPreference = $currErrorActionPreference
}

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
using System;
using System.Text;
using System.Runtime.InteropServices;

namespace PSCloudbase
{
	public sealed class Win32CryptApi
	{
		public static long CRYPT_SILENT = 0x00000040;
		public static long CRYPT_VERIFYCONTEXT = 0xF0000000;
		public static int PROV_RSA_FULL = 1;

		[DllImport("advapi32.dll", CharSet=CharSet.Auto, SetLastError=true)]
		[return : MarshalAs(UnmanagedType.Bool)]
		public static extern bool CryptAcquireContext(ref IntPtr hProv,
													  StringBuilder pszContainer, // Don't use string, as Powershell replaces $null with an empty string
													  StringBuilder pszProvider, // Don't use string, as Powershell replaces $null with an empty string
													  uint dwProvType,
													  uint dwFlags);

		[DllImport("Advapi32.dll", EntryPoint = "CryptReleaseContext", CharSet = CharSet.Unicode, SetLastError = true)]
		public static extern bool CryptReleaseContext(IntPtr hProv, Int32 dwFlags);

		[DllImport("advapi32.dll", SetLastError=true)]
		public static extern bool CryptGenRandom(IntPtr hProv, uint dwLen, byte[] pbBuffer);

		[DllImport("Kernel32.dll")]
		public static extern uint GetLastError();
	}
}
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
/*
Original sources available at: https://bitbucket.org/splatteredbits/carbon
*/

using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;

namespace PSCarbon
{
	public sealed class Lsa
	{
		// ReSharper disable InconsistentNaming
		[StructLayout(LayoutKind.Sequential)]
		internal struct LSA_UNICODE_STRING
		{
			internal LSA_UNICODE_STRING(string inputString)
			{
				if (inputString == null)
				{
					Buffer = IntPtr.Zero;
					Length = 0;
					MaximumLength = 0;
				}
				else
				{
					Buffer = Marshal.StringToHGlobalAuto(inputString);
					Length = (ushort)(inputString.Length * UnicodeEncoding.CharSize);
					MaximumLength = (ushort)((inputString.Length + 1) * UnicodeEncoding.CharSize);
				}
			}

			internal ushort Length;
			internal ushort MaximumLength;
			internal IntPtr Buffer;
		}

		[StructLayout(LayoutKind.Sequential)]
		internal struct LSA_OBJECT_ATTRIBUTES
		{
			internal uint Length;
			internal IntPtr RootDirectory;
			internal LSA_UNICODE_STRING ObjectName;
			internal uint Attributes;
			internal IntPtr SecurityDescriptor;
			internal IntPtr SecurityQualityOfService;
		}

		[StructLayout(LayoutKind.Sequential)]
		public struct LUID
		{
			public uint LowPart;
			public int HighPart;
		}

		// ReSharper disable UnusedMember.Local
		private const uint POLICY_VIEW_LOCAL_INFORMATION = 0x00000001;
		private const uint POLICY_VIEW_AUDIT_INFORMATION = 0x00000002;
		private const uint POLICY_GET_PRIVATE_INFORMATION = 0x00000004;
		private const uint POLICY_TRUST_ADMIN = 0x00000008;
		private const uint POLICY_CREATE_ACCOUNT = 0x00000010;
		private const uint POLICY_CREATE_SECRET = 0x00000014;
		private const uint POLICY_CREATE_PRIVILEGE = 0x00000040;
		private const uint POLICY_SET_DEFAULT_QUOTA_LIMITS = 0x00000080;
		private const uint POLICY_SET_AUDIT_REQUIREMENTS = 0x00000100;
		private const uint POLICY_AUDIT_LOG_ADMIN = 0x00000200;
		private const uint POLICY_SERVER_ADMIN = 0x00000400;
		private const uint POLICY_LOOKUP_NAMES = 0x00000800;
		private const uint POLICY_NOTIFICATION = 0x00001000;
		// ReSharper restore UnusedMember.Local

		[DllImport("advapi32.dll", CharSet = CharSet.Auto, SetLastError = true)]
		public static extern bool LookupPrivilegeValue(
			[MarshalAs(UnmanagedType.LPTStr)] string lpSystemName,
			[MarshalAs(UnmanagedType.LPTStr)] string lpName,
			out LUID lpLuid);

		[DllImport("advapi32.dll", CharSet = CharSet.Unicode)]
		private static extern uint LsaAddAccountRights(
			IntPtr PolicyHandle,
			IntPtr AccountSid,
			LSA_UNICODE_STRING[] UserRights,
			uint CountOfRights);

		[DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = false)]
		private static extern uint LsaClose(IntPtr ObjectHandle);

		[DllImport("advapi32.dll", SetLastError = true)]
		private static extern uint LsaEnumerateAccountRights(IntPtr PolicyHandle,
			IntPtr AccountSid,
			out IntPtr UserRights,
			out uint CountOfRights
			);

		[DllImport("advapi32.dll", SetLastError = true)]
		private static extern uint LsaFreeMemory(IntPtr pBuffer);

		[DllImport("advapi32.dll")]
		private static extern int LsaNtStatusToWinError(long status);

		[DllImport("advapi32.dll", SetLastError = true, PreserveSig = true)]
		private static extern uint LsaOpenPolicy(ref LSA_UNICODE_STRING SystemName, ref LSA_OBJECT_ATTRIBUTES ObjectAttributes, uint DesiredAccess, out IntPtr PolicyHandle );

		[DllImport("advapi32.dll", SetLastError = true, PreserveSig = true)]
		static extern uint LsaRemoveAccountRights(
			IntPtr PolicyHandle,
			IntPtr AccountSid,
			[MarshalAs(UnmanagedType.U1)]
			bool AllRights,
			LSA_UNICODE_STRING[] UserRights,
			uint CountOfRights);
		// ReSharper restore InconsistentNaming

		private static IntPtr GetIdentitySid(string identity)
		{
			var sid =
				new NTAccount(identity).Translate(typeof (SecurityIdentifier)) as SecurityIdentifier;
			if (sid == null)
			{
				throw new ArgumentException(string.Format("Account {0} not found.", identity));
			}
			var sidBytes = new byte[sid.BinaryLength];
			sid.GetBinaryForm(sidBytes, 0);
			var sidPtr = Marshal.AllocHGlobal(sidBytes.Length);
			Marshal.Copy(sidBytes, 0, sidPtr, sidBytes.Length);
			return sidPtr;
		}

		private static IntPtr GetLsaPolicyHandle()
		{
			var computerName = Environment.MachineName;
			IntPtr hPolicy;
			var objectAttributes = new LSA_OBJECT_ATTRIBUTES
			{
				Length = 0,
				RootDirectory = IntPtr.Zero,
				Attributes = 0,
				SecurityDescriptor = IntPtr.Zero,
				SecurityQualityOfService = IntPtr.Zero
			};

			const uint ACCESS_MASK = POLICY_CREATE_SECRET | POLICY_LOOKUP_NAMES | POLICY_VIEW_LOCAL_INFORMATION;
			var machineNameLsa = new LSA_UNICODE_STRING(computerName);
			var result = LsaOpenPolicy(ref machineNameLsa, ref objectAttributes, ACCESS_MASK, out hPolicy);
			HandleLsaResult(result);
			return hPolicy;
		}

		public static string[] GetPrivileges(string identity)
		{
			var sidPtr = GetIdentitySid(identity);
			var hPolicy = GetLsaPolicyHandle();
			var rightsPtr = IntPtr.Zero;

			try
			{

				var privileges = new List<string>();

				uint rightsCount;
				var result = LsaEnumerateAccountRights(hPolicy, sidPtr, out rightsPtr, out rightsCount);
				var win32ErrorCode = LsaNtStatusToWinError(result);
				// the user has no privileges
				if( win32ErrorCode == STATUS_OBJECT_NAME_NOT_FOUND )
				{
					return new string[0];
				}
				HandleLsaResult(result);

				var myLsaus = new LSA_UNICODE_STRING();
				for (ulong i = 0; i < rightsCount; i++)
				{
					var itemAddr = new IntPtr(rightsPtr.ToInt64() + (long) (i*(ulong) Marshal.SizeOf(myLsaus)));
					myLsaus = (LSA_UNICODE_STRING) Marshal.PtrToStructure(itemAddr, myLsaus.GetType());
					var cvt = new char[myLsaus.Length/UnicodeEncoding.CharSize];
					Marshal.Copy(myLsaus.Buffer, cvt, 0, myLsaus.Length/UnicodeEncoding.CharSize);
					var thisRight = new string(cvt);
					privileges.Add(thisRight);
				}
				return privileges.ToArray();
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				var result = LsaClose(hPolicy);
				HandleLsaResult(result);
				result = LsaFreeMemory(rightsPtr);
				HandleLsaResult(result);
			}
		}

		public static void GrantPrivileges(string identity, string[] privileges)
		{
			var sidPtr = GetIdentitySid(identity);
			var hPolicy = GetLsaPolicyHandle();

			try
			{
				var lsaPrivileges = StringsToLsaStrings(privileges);
				var result = LsaAddAccountRights(hPolicy, sidPtr, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				var result = LsaClose(hPolicy);
				HandleLsaResult(result);
			}
		}

		const int STATUS_SUCCESS = 0x0;
		const int STATUS_OBJECT_NAME_NOT_FOUND = 0x00000002;
		const int STATUS_ACCESS_DENIED = 0x00000005;
		const int STATUS_INVALID_HANDLE = 0x00000006;
		const int STATUS_UNSUCCESSFUL = 0x0000001F;
		const int STATUS_INVALID_PARAMETER = 0x00000057;
		const int STATUS_NO_SUCH_PRIVILEGE = 0x00000521;
		const int STATUS_INVALID_SERVER_STATE = 0x00000548;
		const int STATUS_INTERNAL_DB_ERROR = 0x00000567;
		const int STATUS_INSUFFICIENT_RESOURCES = 0x000005AA;

		private static Dictionary<int, string> ErrorMessages = new Dictionary<int, string>
									{
										{STATUS_OBJECT_NAME_NOT_FOUND, "Object name not found. An object in the LSA policy database was not found. The object may have been specified either by SID or by name, depending on its type."},
										{STATUS_ACCESS_DENIED, "Access denied. Caller does not have the appropriate access to complete the operation."},
										{STATUS_INVALID_HANDLE, "Invalid handle. Indicates an object or RPC handle is not valid in the context used."},
										{STATUS_UNSUCCESSFUL, "Unsuccessful. Generic failure, such as RPC connection failure."},
										{STATUS_INVALID_PARAMETER, "Invalid parameter. One of the parameters is not valid."},
										{STATUS_NO_SUCH_PRIVILEGE, "No such privilege. Indicates a specified privilege does not exist."},
										{STATUS_INVALID_SERVER_STATE, "Invalid server state. Indicates the LSA server is currently disabled."},
										{STATUS_INTERNAL_DB_ERROR, "Internal database error. The LSA database contains an internal inconsistency."},
										{STATUS_INSUFFICIENT_RESOURCES, "Insufficient resources. There are not enough system resources (such as memory to allocate buffers) to complete the call."}
									};

		private static void HandleLsaResult(uint returnCode)
		{
			var win32ErrorCode = LsaNtStatusToWinError(returnCode);

			if( win32ErrorCode == STATUS_SUCCESS)
				return;

			if( ErrorMessages.ContainsKey(win32ErrorCode) )
			{
				throw new Win32Exception(win32ErrorCode, ErrorMessages[win32ErrorCode]);
			}

			throw new Win32Exception(win32ErrorCode);
		}

		public static void RevokePrivileges(string identity, string[] privileges)
		{
			var sidPtr = GetIdentitySid(identity);
			var hPolicy = GetLsaPolicyHandle();

			try
			{
				var currentPrivileges = GetPrivileges(identity);
				if (currentPrivileges.Length == 0)
				{
					return;
				}
				var lsaPrivileges = StringsToLsaStrings(privileges);
				var result = LsaRemoveAccountRights(hPolicy, sidPtr, false, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				var result = LsaClose(hPolicy);
				HandleLsaResult(result);
			}

		}

		private static LSA_UNICODE_STRING[] StringsToLsaStrings(string[] privileges)
		{
			var lsaPrivileges = new LSA_UNICODE_STRING[privileges.Length];
			for (var idx = 0; idx < privileges.Length; ++idx)
			{
				lsaPrivileges[idx] = new LSA_UNICODE_STRING(privileges[idx]);
			}
			return lsaPrivileges;
		}
	}
}
"@

Add-Type -TypeDefinition $SourcePolicy -Language CSharp

function SetAssignPrimaryTokenPrivilege($UserName)
{
	$privilege = "SeAssignPrimaryTokenPrivilege"
	if (![PSCarbon.Lsa]::GetPrivileges($UserName).Contains($privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

function SetUserLogonAsServiceRights($UserName)
{
	$privilege = "SeServiceLogonRight"
	if (![PSCarbon.Lsa]::GetPrivileges($UserName).Contains($privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

$Source = @"
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Text;

namespace Tarer
{
	public enum EntryType : byte
	{
		File = 0,
		FileObsolete = 0x30,
		HardLink = 0x31,
		SymLink = 0x32,
		CharDevice = 0x33,
		BlockDevice = 0x34,
		Directory = 0x35,
		Fifo = 0x36,
	}

	public interface ITarHeader
	{
		string FileName { get; set; }
		long SizeInBytes { get; set; }
		DateTime LastModification { get; set; }
		int HeaderSize { get; }
		EntryType EntryType { get; set; }
	}

	public class Tar
	{
		private byte[] dataBuffer = new byte[512];
		private UsTarHeader header;
		private Stream inStream;
		private long remainingBytesInFile;

		public Tar(Stream tarredData) {
			inStream = tarredData;
			header = new UsTarHeader();
		}

		public ITarHeader FileInfo
		{
			get { return header; }
		}

		public void ReadToEnd(string destDirectory)
		{
			while (MoveNext())
			{
				string fileNameFromArchive = FileInfo.FileName;
				string totalPath = destDirectory + Path.DirectorySeparatorChar + fileNameFromArchive;
				if(UsTarHeader.IsPathSeparator(fileNameFromArchive[fileNameFromArchive.Length -1]) || FileInfo.EntryType == EntryType.Directory)
				{
					Directory.CreateDirectory(totalPath);
					continue;
				}
				string fileName = Path.GetFileName(totalPath);
				string directory = totalPath.Remove(totalPath.Length - fileName.Length);
				Directory.CreateDirectory(directory);
				using (FileStream file = File.Create(totalPath))
				{
					Read(file);
				}
			}
		}

		public void Read(Stream dataDestination)
		{
			int readBytes;
			byte[] read;
			while ((readBytes = Read(out read)) != -1)
			{
				dataDestination.Write(read, 0, readBytes);
			}
		}

		protected int Read(out byte[] buffer)
		{
			if(remainingBytesInFile == 0)
			{
				buffer = null;
				return -1;
			}
			int align512 = -1;
			long toRead = remainingBytesInFile - 512;

			if (toRead > 0)
			{
				toRead = 512;
			}
			else
			{
				align512 = 512 - (int)remainingBytesInFile;
				toRead = remainingBytesInFile;
			}

			int bytesRead = 0;
			long bytesRemainingToRead = toRead;
			while (bytesRead < toRead && bytesRemainingToRead > 0)
			{
				bytesRead = inStream.Read(dataBuffer, (int)(toRead-bytesRemainingToRead), (int)bytesRemainingToRead);
				bytesRemainingToRead -= bytesRead;
				remainingBytesInFile -= bytesRead;
			}

			if(inStream.CanSeek && align512 > 0)
			{
				inStream.Seek(align512, SeekOrigin.Current);
			}
			else
			{
				while(align512 > 0)
				{
					inStream.ReadByte();
					--align512;
				}
			}

			buffer = dataBuffer;
			return bytesRead;
		}

		private static bool IsEmpty(IEnumerable<byte> buffer)
		{
			foreach(byte b in buffer)
			{
				if (b != 0)
				{
					return false;
				}
			}
			return true;
		}

		public bool MoveNext()
		{
			byte[] bytes = header.GetBytes();
			int headerRead;
			int bytesRemaining = header.HeaderSize;
			while (bytesRemaining > 0)
			{
				headerRead = inStream.Read(bytes, header.HeaderSize - bytesRemaining, bytesRemaining);
				bytesRemaining -= headerRead;
				if (headerRead <= 0 && bytesRemaining > 0)
				{
					throw new Exception("Error reading tar header. Header size invalid");
				}
			}

			if(IsEmpty(bytes))
			{
				bytesRemaining = header.HeaderSize;
				while (bytesRemaining > 0)
				{
					headerRead = inStream.Read(bytes, header.HeaderSize - bytesRemaining, bytesRemaining);
					bytesRemaining -= headerRead;
					if (headerRead <= 0 && bytesRemaining > 0)
					{
						throw new Exception("Broken archive");
					}
				}
				if (bytesRemaining == 0 && IsEmpty(bytes))
				{
					return false;
				}
				throw new Exception("Error occured: expected end of archive");
			}

			if (!header.UpdateHeaderFromBytes())
			{
				throw new Exception("Checksum check failed");
			}

			remainingBytesInFile = header.SizeInBytes;
			return true;
		}
	}

	internal class TarHeader : ITarHeader
	{
		private byte[] buffer = new byte[512];
		private long headerChecksum;

		private string fileName;
		protected DateTime dateTime1970 = new DateTime(1970, 1, 1, 0, 0, 0);
		public EntryType EntryType { get; set; }
		private static byte[] spaces = Encoding.ASCII.GetBytes("        ");

		public virtual string FileName
		{
			get { return fileName.Replace("\0",string.Empty); }
			set { fileName = value; }
		}

		public long SizeInBytes { get; set; }

		public string SizeString { get { return Convert.ToString(SizeInBytes, 8).PadLeft(11, '0'); } }

		public DateTime LastModification { get; set; }

		public virtual int HeaderSize { get { return 512; } }

		public byte[] GetBytes()
		{
			return buffer;
		}

		public virtual bool UpdateHeaderFromBytes()
		{
			FileName = Encoding.UTF8.GetString(buffer, 0, 100);

			EntryType = (EntryType)buffer[156];

			if((buffer[124] & 0x80) == 0x80) // if size in binary
			{
				long sizeBigEndian = BitConverter.ToInt64(buffer,0x80);
				SizeInBytes = IPAddress.NetworkToHostOrder(sizeBigEndian);
			}
			else
			{
				SizeInBytes = Convert.ToInt64(Encoding.ASCII.GetString(buffer, 124, 11).Trim(), 8);
			}
			long unixTimeStamp = Convert.ToInt64(Encoding.ASCII.GetString(buffer,136,11).Trim(),8);
			LastModification = dateTime1970.AddSeconds(unixTimeStamp);

			var storedChecksum = Convert.ToInt64(Encoding.ASCII.GetString(buffer,148,6).Trim(), 8);
			RecalculateChecksum(buffer);
			if (storedChecksum == headerChecksum)
			{
				return true;
			}

			RecalculateAltChecksum(buffer);
			return storedChecksum == headerChecksum;
		}

		private void RecalculateAltChecksum(byte[] buf)
		{
			spaces.CopyTo(buf, 148);
			headerChecksum = 0;
			foreach(byte b in buf)
			{
				if((b & 0x80) == 0x80)
				{
					headerChecksum -= b ^ 0x80;
				}
				else
				{
					headerChecksum += b;
				}
			}
		}

		protected virtual void RecalculateChecksum(byte[] buf)
		{
			// Set default value for checksum. That is 8 spaces.
			spaces.CopyTo(buf, 148);
			// Calculate checksum
			headerChecksum = 0;
			foreach (byte b in buf)
			{
				headerChecksum += b;
			}
		}
	}
	internal class UsTarHeader : TarHeader
	{
		private const string magic = "ustar";
		private const string version = "  ";

		private string namePrefix = string.Empty;

		public override string FileName
		{
			get { return namePrefix.Replace("\0", string.Empty) + base.FileName.Replace("\0", string.Empty); }
			set
			{
				if (value.Length > 255)
				{
					throw new Exception("UsTar fileName can not be longer than 255 chars");
				}
				if (value.Length > 100)
				{
				int position = value.Length - 100;
				while (!IsPathSeparator(value[position]))
				{
					++position;
					if (position == value.Length)
					{
						break;
					}
				}
				if (position == value.Length)
				{
					position = value.Length - 100;
				}
				namePrefix = value.Substring(0, position);
				base.FileName = value.Substring(position, value.Length - position);
				}
				else
				{
					base.FileName = value;
				}
			}
		}

		public override bool UpdateHeaderFromBytes()
		{
			byte[] bytes = GetBytes();
			namePrefix = Encoding.UTF8.GetString(bytes, 347, 157);
			return base.UpdateHeaderFromBytes();
		}

		internal static bool IsPathSeparator(char ch)
		{
			return (ch == '\\' || ch == '/' || ch == '|');
		}
	}
}
"@

Add-Type -TypeDefinition $Source -Language CSharp

Function GUnZip-File{
	Param(
		$infile,
		$outdir
		)

	$input = New-Object System.IO.FileStream $inFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
	$tempFile = "$env:TEMP\jujud.tar"
	$tempOut = New-Object System.IO.FileStream $tempFile, ([IO.FileMode]::Create), ([IO.FileAccess]::Write), ([IO.FileShare]::None)
	$gzipStream = New-Object System.IO.Compression.GzipStream $input, ([IO.Compression.CompressionMode]::Decompress)

	$buffer = New-Object byte[](1024)
	while($true){
		$read = $gzipstream.Read($buffer, 0, 1024)
		if ($read -le 0){break}
		$tempOut.Write($buffer, 0, $read)
	}
	$gzipStream.Close()
	$tempOut.Close()
	$input.Close()

	$in = New-Object System.IO.FileStream $tempFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
	$tar = New-Object Tarer.Tar($in)
	$tar.ReadToEnd($outdir)
	$in.Close()
	rm $tempFile
}

Function Get-FileSHA256{
	Param(
		$FilePath
	)
	$hash = [Security.Cryptography.HashAlgorithm]::Create( "SHA256" )
	$stream = ([IO.StreamReader]$FilePath).BaseStream
	$res = -join ($hash.ComputeHash($stream) | ForEach { "{0:x2}" -f $_ })
	$stream.Close()
	return $res
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


mkdir -Force "C:\Juju"
mkdir C:\Juju\tmp
mkdir "C:\Juju\bin"
mkdir "C:\Juju\lib\juju\locks"
$adminsGroup = (New-Object System.Security.Principal.SecurityIdentifier("S-1-5-32-544")).Translate([System.Security.Principal.NTAccount])
$acl = Get-Acl -Path 'C:\Juju'
$acl.SetAccessRuleProtection($true, $false)
$adminPerm = "$adminsGroup", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
$jujudPerm = "jujud", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule $adminPerm
$acl.AddAccessRule($rule)
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule $jujudPerm
$acl.AddAccessRule($rule)
Set-Acl -Path 'C:\Juju' -AclObject $acl
setx /m PATH "$env:PATH;C:\Juju\bin\"
Set-Content "C:\Juju\lib\juju\nonce.txt" "'FAKE_NONCE'"
$binDir="C:\Juju\lib\juju\tools\1.2.3-win8-amd64"
mkdir 'C:\Juju\log\juju'
mkdir $binDir
$WebClient = New-Object System.Net.WebClient
[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}
ExecRetry { $WebClient.DownloadFile('http://foo.com/tools/released/juju1.2.3-win8-amd64.tgz', "$binDir\tools.tar.gz") }
$dToolsHash = Get-FileSHA256 -FilePath "$binDir\tools.tar.gz"
$dToolsHash > "$binDir\juju1.2.3-win8-amd64.sha256"
if ($dToolsHash.ToLower() -ne "1234"){ Throw "Tools checksum mismatch"}
GUnZip-File -infile $binDir\tools.tar.gz -outdir $binDir
rm "$binDir\tools.tar*"
Set-Content $binDir\downloaded-tools.txt '{"version":"1.2.3-win8-amd64","url":"http://foo.com/tools/released/juju1.2.3-win8-amd64.tgz","sha256":"1234","size":10}'
New-Item -Path 'HKLM:\SOFTWARE\juju-core'
$acl = Get-Acl -Path 'HKLM:\SOFTWARE\juju-core'
$acl.SetAccessRuleProtection($true, $false)
$adminPerm = "$adminsGroup", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
$jujudPerm = "jujud", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
$rule = New-Object System.Security.AccessControl.RegistryAccessRule $adminPerm
$acl.AddAccessRule($rule)
$rule = New-Object System.Security.AccessControl.RegistryAccessRule $jujudPerm
$acl.AddAccessRule($rule)
Set-Acl -Path 'HKLM:\SOFTWARE\juju-core' -AclObject $acl
New-ItemProperty -Path 'HKLM:\SOFTWARE\juju-core' -Name 'JUJU_DEV_FEATURE_FLAGS'
Set-ItemProperty -Path 'HKLM:\SOFTWARE\juju-core' -Name 'JUJU_DEV_FEATURE_FLAGS' -Value ''
mkdir 'C:\Juju\lib\juju\agents\machine-10'
Set-Content 'C:/Juju/lib/juju/agents/machine-10/agent.conf' @"
# format 1.18
tag: machine-10
datadir: C:/Juju/lib/juju
logdir: C:/Juju/log/juju
metricsspooldir: C:/Juju/lib/juju/metricspool
nonce: FAKE_NONCE
jobs:
- JobHostUnits
upgradedToVersion: 1.2.3
cacert: |
  CA CERT
  SERVER CERT
  -----BEGIN CERTIFICATE-----
  MIIBdzCCASOgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
  MAsGA1UEAxMEcm9vdDAeFw0xMjExMDgxNjIyMzRaFw0xMzExMDgxNjI3MzRaMBwx
  DDAKBgNVBAoTA2htbTEMMAoGA1UEAxMDYW55MFowCwYJKoZIhvcNAQEBA0sAMEgC
  QQCACqz6JPwM7nbxAWub+APpnNB7myckWJ6nnsPKi9SipP1hyhfzkp8RGMJ5Uv7y
  8CSTtJ8kg/ibka1VV8LvP9tnAgMBAAGjUjBQMA4GA1UdDwEB/wQEAwIAsDAdBgNV
  HQ4EFgQU6G1ERaHCgfAv+yoDMFVpDbLOmIQwHwYDVR0jBBgwFoAUP/mfUdwOlHfk
  fR+gLQjslxf64w0wCwYJKoZIhvcNAQEFA0EAbn0MaxWVgGYBomeLYfDdb8vCq/5/
  G/2iCUQCXsVrBparMLFnor/iKOkJB5n3z3rtu70rFt+DpX6L8uBR3LB3+A==
  -----END CERTIFICATE-----
stateaddresses:
- state-addr.testing.invalid:12345
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
apiaddresses:
- state-addr.testing.invalid:54321
oldpassword: arble
values:
  AGENT_SERVICE_NAME: jujud-machine-10
  PROVIDER_TYPE: dummy
mongoversion: "0.0"

"@
cmd.exe /C mklink /D C:\Juju\lib\juju\tools\machine-10 1.2.3-win8-amd64
New-Service -Credential $jujuCreds -Name 'jujud-machine-10' -DependsOn Winmgmt -DisplayName 'juju agent for machine-10' '"C:\Juju\lib\juju\tools\machine-10\jujud.exe" machine --data-dir "C:\Juju\lib\juju" --machine-id 10 --debug'
sc.exe failure 'jujud-machine-10' reset=5 actions=restart/1000
sc.exe failureflag 'jujud-machine-10' 1
Start-Service 'jujud-machine-10'`
