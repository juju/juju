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
			SecurityIdentifier sid =
				new NTAccount(identity).Translate(typeof (SecurityIdentifier)) as SecurityIdentifier;
			if (sid == null)
			{
				throw new ArgumentException(string.Format("Account {0} not found.", identity));
			}
			byte[] sidBytes = new byte[sid.BinaryLength];
			sid.GetBinaryForm(sidBytes, 0);
			System.IntPtr sidPtr = Marshal.AllocHGlobal(sidBytes.Length);
			Marshal.Copy(sidBytes, 0, sidPtr, sidBytes.Length);
			return sidPtr;
		}

		private static IntPtr GetLsaPolicyHandle()
		{
			string computerName = Environment.MachineName;
			IntPtr hPolicy;
			LSA_OBJECT_ATTRIBUTES objectAttributes = new LSA_OBJECT_ATTRIBUTES();
			objectAttributes.Length = 0;
			objectAttributes.RootDirectory = IntPtr.Zero;
			objectAttributes.Attributes = 0;
			objectAttributes.SecurityDescriptor = IntPtr.Zero;
			objectAttributes.SecurityQualityOfService = IntPtr.Zero;

			const uint ACCESS_MASK = POLICY_CREATE_SECRET | POLICY_LOOKUP_NAMES | POLICY_VIEW_LOCAL_INFORMATION;
			LSA_UNICODE_STRING machineNameLsa = new LSA_UNICODE_STRING(computerName);
			uint result = LsaOpenPolicy(ref machineNameLsa, ref objectAttributes, ACCESS_MASK, out hPolicy);
			HandleLsaResult(result);
			return hPolicy;
		}

		public static string[] GetPrivileges(string identity)
		{
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();
			IntPtr rightsPtr = IntPtr.Zero;

			try
			{

				List<string> privileges = new List<string>();

				uint rightsCount;
				uint result = LsaEnumerateAccountRights(hPolicy, sidPtr, out rightsPtr, out rightsCount);
				int win32ErrorCode = LsaNtStatusToWinError(result);
				// the user has no privileges
				if( win32ErrorCode == STATUS_OBJECT_NAME_NOT_FOUND )
				{
					return new string[0];
				}
				HandleLsaResult(result);

				LSA_UNICODE_STRING myLsaus = new LSA_UNICODE_STRING();
				for (ulong i = 0; i < rightsCount; i++)
				{
					IntPtr itemAddr = new IntPtr(rightsPtr.ToInt64() + (long) (i*(ulong) Marshal.SizeOf(myLsaus)));
					myLsaus = (LSA_UNICODE_STRING) Marshal.PtrToStructure(itemAddr, myLsaus.GetType());
					char[] cvt = new char[myLsaus.Length/UnicodeEncoding.CharSize];
					Marshal.Copy(myLsaus.Buffer, cvt, 0, myLsaus.Length/UnicodeEncoding.CharSize);
					string thisRight = new string(cvt);
					privileges.Add(thisRight);
				}
				return privileges.ToArray();
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
				HandleLsaResult(result);
				result = LsaFreeMemory(rightsPtr);
				HandleLsaResult(result);
			}
		}

		public static void GrantPrivileges(string identity, string[] privileges)
		{
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();

			try
			{
				LSA_UNICODE_STRING[] lsaPrivileges = StringsToLsaStrings(privileges);
				uint result = LsaAddAccountRights(hPolicy, sidPtr, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
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

		private static Dictionary<int, string> ErrorMessages = new Dictionary<int, string>();
		public Lsa () {
			ErrorMessages.Add(STATUS_ACCESS_DENIED, "Access denied. Caller does not have the appropriate access to complete the operation.");
			ErrorMessages.Add(STATUS_INVALID_HANDLE, "Invalid handle. Indicates an object or RPC handle is not valid in the context used.");
			ErrorMessages.Add(STATUS_UNSUCCESSFUL, "Unsuccessful. Generic failure, such as RPC connection failure.");
			ErrorMessages.Add(STATUS_INVALID_PARAMETER, "Invalid parameter. One of the parameters is not valid.");
			ErrorMessages.Add(STATUS_NO_SUCH_PRIVILEGE, "No such privilege. Indicates a specified privilege does not exist.");
			ErrorMessages.Add(STATUS_INVALID_SERVER_STATE, "Invalid server state. Indicates the LSA server is currently disabled.");
			ErrorMessages.Add(STATUS_INTERNAL_DB_ERROR, "Internal database error. The LSA database contains an internal inconsistency.");
			ErrorMessages.Add(STATUS_INSUFFICIENT_RESOURCES, "Insufficient resources. There are not enough system resources (such as memory to allocate buffers) to complete the call.");
			ErrorMessages.Add(STATUS_OBJECT_NAME_NOT_FOUND, "Object name not found. An object in the LSA policy database was not found. The object may have been specified either by SID or by name, depending on its type.");
		}

		private static void HandleLsaResult(uint returnCode)
		{
			int win32ErrorCode = LsaNtStatusToWinError(returnCode);

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
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();

			try
			{
				string[] currentPrivileges = GetPrivileges(identity);
				if (currentPrivileges.Length == 0)
				{
					return;
				}
				LSA_UNICODE_STRING[] lsaPrivileges = StringsToLsaStrings(privileges);
				uint result = LsaRemoveAccountRights(hPolicy, sidPtr, false, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
				HandleLsaResult(result);
			}

		}

		private static LSA_UNICODE_STRING[] StringsToLsaStrings(string[] privileges)
		{
			LSA_UNICODE_STRING[] lsaPrivileges = new LSA_UNICODE_STRING[privileges.Length];
			for (int idx = 0; idx < privileges.Length; ++idx)
			{
				lsaPrivileges[idx] = new LSA_UNICODE_STRING(privileges[idx]);
			}
			return lsaPrivileges;
		}
	}
}
