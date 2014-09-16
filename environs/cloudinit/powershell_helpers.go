package cloudinit

var winPowershellHelperFunctions = `

$ErrorActionPreference = "Stop"

function ExecRetry($command, $maxRetryCount = 10, $retryInterval=2)
{
    $currErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"

    $retryCount = 0
    while ($true)
    {
        try
        {
            & $command
            break
        }
        catch [System.Exception]
        {
            $retryCount++
            if ($retryCount -ge $maxRetryCount)
            {
                $ErrorActionPreference = $currErrorActionPreference
                throw
            }
            else
            {
                Write-Error $_.Exception
                Start-Sleep $retryInterval
            }
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

	$objOU = [ADSI]"WinNT://$hostname/Administrators,group"
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
        public static long CRYPT_SILENT                     = 0x00000040;
        public static long CRYPT_VERIFYCONTEXT              = 0xF0000000;
        public static int PROV_RSA_FULL                     = 1;

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

        private static readonly Dictionary<int, string> ErrorMessages = new Dictionary<int, string>
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

$ServiceChangeErrors = @{}
$ServiceChangeErrors.Add(1, "Not Supported")
$ServiceChangeErrors.Add(2, "Access Denied")
$ServiceChangeErrors.Add(3, "Dependent Services Running")
$ServiceChangeErrors.Add(4, "Invalid Service Control")
$ServiceChangeErrors.Add(5, "Service Cannot Accept Control")
$ServiceChangeErrors.Add(6, "Service Not Active")
$ServiceChangeErrors.Add(7, "Service Request Timeout")
$ServiceChangeErrors.Add(8, "Unknown Failure")
$ServiceChangeErrors.Add(9, "Path Not Found")
$ServiceChangeErrors.Add(10, "Service Already Running")
$ServiceChangeErrors.Add(11, "Service Database Locked")
$ServiceChangeErrors.Add(12, "Service Dependency Deleted")
$ServiceChangeErrors.Add(13, "Service Dependency Failure")
$ServiceChangeErrors.Add(14, "Service Disabled")
$ServiceChangeErrors.Add(15, "Service Logon Failure")
$ServiceChangeErrors.Add(16, "Service Marked For Deletion")
$ServiceChangeErrors.Add(17, "Service No Thread")
$ServiceChangeErrors.Add(18, "Status Circular Dependency")
$ServiceChangeErrors.Add(19, "Status Duplicate Name")
$ServiceChangeErrors.Add(20, "Status Invalid Name")
$ServiceChangeErrors.Add(21, "Status Invalid Parameter")
$ServiceChangeErrors.Add(22, "Status Invalid Service Account")
$ServiceChangeErrors.Add(23, "Status Service Exists")
$ServiceChangeErrors.Add(24, "Service Already Paused")


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
using System.Text;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.ComponentModel;

namespace PSCloudbase
{
    public class ProcessManager
    {
        const int LOGON32_LOGON_SERVICE = 5;
        const int LOGON32_PROVIDER_DEFAULT = 0;
        const int TOKEN_ALL_ACCESS = 0x000f01ff;
        const uint GENERIC_ALL_ACCESS = 0x10000000;
        const uint INFINITE = 0xFFFFFFFF;
        const uint PI_NOUI = 0x00000001;
        const uint WAIT_FAILED = 0xFFFFFFFF;

        enum SECURITY_IMPERSONATION_LEVEL
        {
            SecurityAnonymous,
            SecurityIdentification,
            SecurityImpersonation,
            SecurityDelegation
        }

        enum TOKEN_TYPE
        {
            TokenPrimary = 1,
            TokenImpersonation
        }

        [StructLayout(LayoutKind.Sequential)]
        struct SECURITY_ATTRIBUTES
        {
            public int nLength;
            public IntPtr lpSecurityDescriptor;
            public int bInheritHandle;
        }

        [StructLayout(LayoutKind.Sequential)]
        struct PROCESS_INFORMATION
        {
            public IntPtr hProcess;
            public IntPtr hThread;
            public int dwProcessId;
            public int dwThreadId;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        struct STARTUPINFO
        {
            public Int32 cb;
            public string lpReserved;
            public string lpDesktop;
            public string lpTitle;
            public Int32 dwX;
            public Int32 dwY;
            public Int32 dwXSize;
            public Int32 dwYSize;
            public Int32 dwXCountChars;
            public Int32 dwYCountChars;
            public Int32 dwFillAttribute;
            public Int32 dwFlags;
            public Int16 wShowWindow;
            public Int16 cbReserved2;
            public IntPtr lpReserved2;
            public IntPtr hStdInput;
            public IntPtr hStdOutput;
            public IntPtr hStdError;
        }

        [StructLayout(LayoutKind.Sequential)]
        struct PROFILEINFO {
            public int dwSize;
            public uint dwFlags;
            [MarshalAs(UnmanagedType.LPTStr)]
            public String lpUserName;
            [MarshalAs(UnmanagedType.LPTStr)]
            public String lpProfilePath;
            [MarshalAs(UnmanagedType.LPTStr)]
            public String lpDefaultPath;
            [MarshalAs(UnmanagedType.LPTStr)]
            public String lpServerName;
            [MarshalAs(UnmanagedType.LPTStr)]
            public String lpPolicyPath;
            public IntPtr hProfile;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct USER_INFO_4
        {
            public string name;
            public string password;
            public int password_age;
            public uint priv;
            public string home_dir;
            public string comment;
            public uint flags;
            public string script_path;
            public uint auth_flags;
            public string full_name;
            public string usr_comment;
            public string parms;
            public string workstations;
            public int last_logon;
            public int last_logoff;
            public int acct_expires;
            public int max_storage;
            public int units_per_week;
            public IntPtr logon_hours;    // This is a PBYTE
            public int bad_pw_count;
            public int num_logons;
            public string logon_server;
            public int country_code;
            public int code_page;
            public IntPtr user_sid;     // This is a PSID
            public int primary_group_id;
            public string profile;
            public string home_dir_drive;
            public int password_expired;
        }

        [DllImport("advapi32.dll", CharSet=CharSet.Auto, SetLastError=true)]
        extern static bool DuplicateTokenEx(
            IntPtr hExistingToken,
            uint dwDesiredAccess,
            ref SECURITY_ATTRIBUTES lpTokenAttributes,
            SECURITY_IMPERSONATION_LEVEL ImpersonationLevel,
            TOKEN_TYPE TokenType,
            out IntPtr phNewToken);

        [DllImport("advapi32.dll", SetLastError=true)]
        static extern bool LogonUser(
            string lpszUsername,
            string lpszDomain,
            string lpszPassword,
            int dwLogonType,
            int dwLogonProvider,
            out IntPtr phToken);

        [DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Auto)]
        static extern bool CreateProcessAsUser(
            IntPtr hToken,
            string lpApplicationName,
            string lpCommandLine,
            ref SECURITY_ATTRIBUTES lpProcessAttributes,
            ref SECURITY_ATTRIBUTES lpThreadAttributes,
            bool bInheritHandles,
            uint dwCreationFlags,
            IntPtr lpEnvironment,
            string lpCurrentDirectory,
            ref STARTUPINFO lpStartupInfo,
            out PROCESS_INFORMATION lpProcessInformation);

        [DllImport("kernel32.dll", SetLastError=true)]
        static extern UInt32 WaitForSingleObject(IntPtr hHandle,
                                                 UInt32 dwMilliseconds);

        [DllImport("Kernel32.dll")]
        static extern int GetLastError();

        [DllImport("Kernel32.dll")]
        extern static int CloseHandle(IntPtr handle);

        [DllImport("kernel32.dll", SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        static extern bool GetExitCodeProcess(IntPtr hProcess,
                                              out uint lpExitCode);

        [DllImport("userenv.dll", SetLastError=true, CharSet=CharSet.Auto)]
        [return: MarshalAs(UnmanagedType.Bool)]
        static extern bool LoadUserProfile(IntPtr hToken,
                                           ref PROFILEINFO lpProfileInfo);

        [DllImport("userenv.dll", SetLastError=true, CharSet=CharSet.Auto)]
        [return: MarshalAs(UnmanagedType.Bool)]
        static extern bool UnloadUserProfile(IntPtr hToken, IntPtr hProfile);

         [DllImport("Netapi32.dll", CharSet=CharSet.Unicode, ExactSpelling=true)]
        extern static int NetUserGetInfo(
            [MarshalAs(UnmanagedType.LPWStr)] string ServerName,
            [MarshalAs(UnmanagedType.LPWStr)] string UserName,
            int level, out IntPtr BufPtr);

        public static uint RunProcess(string userName, string password,
                                      string domain, string cmd,
                                      string arguments,
                                      bool loadUserProfile = true)
        {
            bool retValue;
            IntPtr phToken = IntPtr.Zero;
            IntPtr phTokenDup = IntPtr.Zero;
            PROCESS_INFORMATION pInfo = new PROCESS_INFORMATION();
            PROFILEINFO pi = new PROFILEINFO();

            try
            {
                retValue = LogonUser(userName, domain, password,
                                     LOGON32_LOGON_SERVICE,
                                     LOGON32_PROVIDER_DEFAULT,
                                     out phToken);
                if(!retValue)
                    throw new Win32Exception(GetLastError());

                var sa = new SECURITY_ATTRIBUTES();
                sa.nLength = Marshal.SizeOf(sa);

                retValue = DuplicateTokenEx(
                    phToken, GENERIC_ALL_ACCESS, ref sa,
                    SECURITY_IMPERSONATION_LEVEL.SecurityImpersonation,
                    TOKEN_TYPE.TokenPrimary, out phTokenDup);
                if(!retValue)
                    throw new Win32Exception(GetLastError());

                STARTUPINFO sInfo = new STARTUPINFO();
                sInfo.lpDesktop = "";

                if(loadUserProfile)
                {
                    IntPtr userInfoPtr = IntPtr.Zero;
                    int retValueNetUser = NetUserGetInfo(null, userName, 4,
                                                         out userInfoPtr);
                    if(retValueNetUser != 0)
                        throw new Win32Exception(retValueNetUser);

                    USER_INFO_4 userInfo = (USER_INFO_4)Marshal.PtrToStructure(
                        userInfoPtr, typeof(USER_INFO_4));

                    pi.dwSize = Marshal.SizeOf(pi);
                    pi.dwFlags = PI_NOUI;
                    pi.lpUserName = userName;
                    pi.lpProfilePath = userInfo.profile;

                    retValue = LoadUserProfile(phTokenDup, ref pi);
                    if(!retValue)
                        throw new Win32Exception(GetLastError());
                }

                retValue = CreateProcessAsUser(phTokenDup, cmd, arguments,
                                               ref sa, ref sa, false, 0,
                                               IntPtr.Zero, null,
                                               ref sInfo, out pInfo);
                if(!retValue)
                    throw new Win32Exception(GetLastError());

                if(WaitForSingleObject(pInfo.hProcess, INFINITE) == WAIT_FAILED)
                    throw new Win32Exception(GetLastError());

                uint exitCode;
                retValue = GetExitCodeProcess(pInfo.hProcess, out exitCode);
                if(!retValue)
                    throw new Win32Exception(GetLastError());

                return exitCode;
            }
            finally
            {
                if(pi.hProfile != IntPtr.Zero)
                    UnloadUserProfile(phTokenDup, pi.hProfile);
                if(phToken != IntPtr.Zero)
                    CloseHandle(phToken);
                if(phTokenDup != IntPtr.Zero)
                    CloseHandle(phTokenDup);
                if(pInfo.hProcess != IntPtr.Zero)
                    CloseHandle(pInfo.hProcess);
            }
        }
    }
}
"@

Add-Type -TypeDefinition $Source -Language CSharp

function Start-ProcessAsUser
{
    [CmdletBinding()]
    param
    (
        [parameter(Mandatory=$true, ValueFromPipeline=$true)]
        [string]$Command,

        [parameter()]
        [string]$Arguments,

        [parameter(Mandatory=$true)]
        [PSCredential]$Credential,

        [parameter()]
        [bool]$LoadUserProfile = $true
    )
    process
    {
        $nc = $Credential.GetNetworkCredential()

        $domain = "."
        if($nc.Domain)
        {
            $domain = $nc.Domain
        }

        [PSCloudbase.ProcessManager]::RunProcess($nc.UserName, $nc.Password,
                                                 $domain, $Command,
                                                 $Arguments, $LoadUserProfile)
    }
}

$powershell = "$ENV:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
$cmdExe = "$ENV:SystemRoot\System32\cmd.exe"

$juju_passwd = Get-RandomPassword 20
$juju_passwd += "^"
create-account jujud "Juju Admin user" $juju_passwd
$hostname = hostname
$juju_user = "$hostname\jujud"

SetUserLogonAsServiceRights $juju_user
SetAssignPrimaryTokenPrivilege $juju_user

New-ItemProperty "HKLM:\Software\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList" -Name "jujud" -Value 0 -PropertyType "DWord" 

$secpasswd = ConvertTo-SecureString $juju_passwd -AsPlainText -Force
$jujuCreds = New-Object System.Management.Automation.PSCredential ($juju_user, $secpasswd)

`

var winSetPasswdScript = `

Set-Content "C:\juju\bin\save_pass.ps1" @"
Param (
	[Parameter(Mandatory=` + "`$true" + `)]
	[string]` + "`$pass" + `
)

` + "`$secpasswd" + ` = ConvertTo-SecureString ` + "`$pass" + ` -AsPlainText -Force
` + "`$secpasswd" + ` | convertfrom-securestring | Add-Content C:\Juju\Jujud.pass 
"@

`
