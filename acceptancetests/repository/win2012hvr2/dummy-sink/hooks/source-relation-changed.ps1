$path = join-path $ENV:ProgramData dummy-sink
mkdir $path
$tokenpath = join-path $path token
relation-get token > $tokenpath
