#ps1_sysnative
$userdata=@"
%s
"@

Function Decode-Base64 {
	Param(
		$inFile,
		$outFile
	)
	$bufferSize = 9000 # should be a multiplier of 4
	$buffer = New-Object char[] $bufferSize

	$reader = [System.IO.File]::OpenText($inFile)
	$writer = [System.IO.File]::OpenWrite($outFile)

	$bytesRead = 0
	do
	{
		$bytesRead = $reader.Read($buffer, 0, $bufferSize);
		$bytes = [Convert]::FromBase64CharArray($buffer, 0, $bytesRead);
		$writer.Write($bytes, 0, $bytes.Length);
	} while ($bytesRead -eq $bufferSize);

	$reader.Dispose()
	$writer.Dispose()
}

Function GUnZip-File {
	Param(
		$inFile,
		$outFile
	)
	$in = New-Object System.IO.FileStream $inFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
	$out = New-Object System.IO.FileStream $outFile, ([IO.FileMode]::Create), ([IO.FileAccess]::Write), ([IO.FileShare]::None)
	$gzipStream = New-Object System.IO.Compression.GZipStream $in, ([IO.Compression.CompressionMode]::Decompress)
	$buffer = New-Object byte[](1024)
	while($true){
		$read = $gzipstream.Read($buffer, 0, 1024)
		if ($read -le 0){break}
		$out.Write($buffer, 0, $read)
	}
	$gzipStream.Close()
	$out.Close()
	$in.Close()
}

$b64File = "$env:TEMP\juju\udata.b64"
$gzFile = "$env:TEMP\juju\udata.gz"
$udataScript = "$env:TEMP\juju\udata.ps1"
mkdir "$env:TEMP\juju"

Set-Content $b64File $userdata
Decode-Base64 -inFile $b64File -outFile $gzFile
GUnZip-File -inFile $gzFile -outFile $udataScript

& $udataScript

rm -Recurse "$env:TEMP\juju"

