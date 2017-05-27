status-set.exe "maintenance" "Updating token"
$path = join-path $ENV:ProgramData dummy-sink
mkdir $path
$tokenpath = join-path $path token
relation-get token > $tokenpath
$current_token = relation-get token
juju-log.exe "Token is $current_token"
status-set.exe "active" "Token is $current_token"
