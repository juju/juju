
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

# TryExecAll attempts all of the commands in the supplied array until
# one can be executed without throwing an exception. If none of the
# commands succeeds, an exception will be raised.
function TryExecAll($commands)
{
	$currErrorActionPreference = $ErrorActionPreference
	$ErrorActionPreference = "Continue"

	foreach ($command in $commands)
	{
		try
		{
			& $command
			$ErrorActionPreference = $currErrorActionPreference
			return
		}
		catch [System.Exception]
		{
			Write-Error $_.Exception
		}
	}

	$ErrorActionPreference = $currErrorActionPreference
	throw "All commands failed"
}
