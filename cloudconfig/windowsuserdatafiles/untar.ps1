
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
    while($true) {
        $read = $gzipstream.Read($buffer, 0, 1024)
        if ($read -le 0){break}
        $tempOut.Write($buffer, 0, $read)
    }
    $gzipStream.Close()
    $tempOut.Close()
    $input.Close()

    $in = New-Object System.IO.FileStream $tempFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
    Untar-File $in $outdir
    $in.Close()
    rm $tempFile
}

$HEADERSIZE = 512

Function Untar-File {
    Param(
        $inStream,
        $outdir
        )
    $DirectoryEntryType = 0x35
    $headerBytes = New-Object byte[]($HEADERSIZE)

    # $headerBytes is written inside, function returns whether we've reached the end
    while(GetHeaderBytes $inStream $headerBytes) {
        $fileName, $entryType, $sizeInBytes = GetFileInfoFromHeader $headerBytes

        $totalPath = Join-Path $outDir $fileName
        if ($entryType -eq $DirectoryEntryType) {
            [System.IO.Directory]::CreateDirectory($totalPath)
            continue;
        }

        $fName = [System.IO.Path]::GetFileName($totalPath)
        $dirName = [System.IO.Path]::GetDirectoryName($totalPath)
        [System.IO.Directory]::CreateDirectory($dirName)
        $file = [System.IO.File]::Create($totalPath)
        WriteTarEntryToFile $inStream $file $sizeInBytes
        $file.Close()
    }
}

Function WriteTarEntryToFile {
    Param(
        $inStream,
        $outFile,
        $sizeInBytes
        )
    $moveToAlign512 = 0
    $toRead = 0
    $buf = New-Object byte[](512)

    $remainingBytesInFile = $sizeInBytes
    while ($remainingBytesInFile -ne 0) {
        if ($remainingBytesInFile - 512 -lt 0) {
            $moveToAlign512 = 512 - $remainingBytesInFile
            $toRead = $remainingBytesInFile
        } else {
            $toRead = 512
        }

        $bytesRead = 0
        $bytesRemainingToRead = $toRead
        while ($bytesRead -lt $toRead -and $bytesRemainingToRead -gt 0) {
            $bytesRead = $inStream.Read($buf, $toRead - $bytesRemainingToRead, $bytesRemainingToRead)
            $bytesRemainingToRead = $bytesRemainingToRead - $bytesRead
            $remainingBytesInFile = $remainingBytesInFile - $bytesRead
            $outFile.Write($buf, 0, $bytesRead)
        }

        if ($moveToAlign512 -ne 0) {
            $inStream.Seek($moveToAlign512, [System.IO.SeekOrigin]::Current)
        }
    }
}

Function GetHeaderBytes {
    Param($inStream, $headerBytes)

    $headerRead = 0
    $bytesRemaining = $HEADERSIZE
    while ($bytesRemaining -gt 0) {
        $headerRead = $inStream.Read($headerBytes, $HEADERSIZE - $bytesRemaining, $bytesRemaining)
        $bytesRemaining -= $headerRead
        if ($headerRead -le 0 -and $bytesRemaining -gt 0) {
            throw "Error reading tar header. Header size invalid"
        }
    }

    # Proper end of archive is 2 empty headers
    if (IsEmptyByteArray $headerBytes) {
        $bytesRemaining = $HEADERSIZE
        while ($bytesRemaining -gt 0) {
            $headerRead = $inStream.Read($headerBytes, $HEADERSIZE - $bytesRemaining, $bytesRemaining)
            $bytesRemaining -= $headerRead
            if ($headerRead -le 0 -and $bytesRemaining -gt 0) {
                throw "Broken end archive"
            }
        }
        if ($bytesRemaining -eq 0 -and (IsEmptyByteArray($headerBytes))) {
            return $false
        }
        throw "Error occurred: expected end of archive"
    }

    return $true
}

Function GetFileInfoFromHeader {
    Param($headerBytes)

    $FileName = [System.Text.Encoding]::UTF8.GetString($headerBytes, 0, 100);
    $EntryType = $headerBytes[156];
    $SizeInBytes = [Convert]::ToInt64([System.Text.Encoding]::ASCII.GetString($headerBytes, 124, 11).Trim(), 8);
    Return $FileName.replace("`0", [String].Empty), $EntryType, $SizeInBytes
}

Function IsEmptyByteArray {
    Param ($bytes)
    foreach($b in $bytes) {
        if ($b -ne 0) {
            return $false
        }
    }
    return $true
}
