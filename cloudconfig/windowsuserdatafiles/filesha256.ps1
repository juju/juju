
Function Get-FileSHA256{
	Param(
		$FilePath
	)
	try {
		$hash = [Security.Cryptography.HashAlgorithm]::Create( "SHA256" )
		$stream = ([IO.StreamReader]$FilePath).BaseStream
		$res = -join ($hash.ComputeHash($stream) | ForEach { "{0:x2}" -f $_ })
		$stream.Close()
		return $res
	} catch [System.Management.Automation.RuntimeException] {
		return (Get-FileHash -Path $FilePath).Hash
	}
}
