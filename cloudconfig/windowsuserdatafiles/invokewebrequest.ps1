
Function Invoke-FastWebRequest {
	Param(
		$URI,
		$OutFile
	)

	if(!([System.Management.Automation.PSTypeName]'System.Net.Http.HttpClient').Type)
	{
		$assembly = [System.Reflection.Assembly]::LoadWithPartialName("System.Net.Http")
	}

	$client = new-object System.Net.Http.HttpClient

	$task = $client.GetStreamAsync($URI)
	$response = $task.Result
	$outStream = New-Object IO.FileStream $OutFile, Create, Write, None

	try {
		$totRead = 0
		$buffer = New-Object Byte[] 1MB
		while (($read = $response.Read($buffer, 0, $buffer.Length)) -gt 0) {
		$totRead += $read
		$outStream.Write($buffer, 0, $read);
		}
	}
	finally {
		$outStream.Close()
	}
}

