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
