# This script is intended to run on Windows. It extracts compressed Juju binary
# in a ZIP archive and run the client-server test.
set -eu

server="$1"
client="$2"
shift 2

/cygdrive/c/progra~2/7-Zip/7z.exe e -y -oserver $server
/cygdrive/c/progra~2/7-Zip/7z.exe e -y -oclient $client

mkdir logs
juju destroy-environment --force -y compatibility-control || true
python C:\\users\\Administrator\\juju-ci-tools\\assess_heterogeneous_control.py \
  server/juju.exe client/juju.exe test-win-client-server \
  compatibility-control logs "$@"