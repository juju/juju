# This script is intended to run on Windows. It extracts compressed Juju binary
# in a ZIP archive and run the client-server test.
set -eux

candidate_version="$1"
old_juju_version="$2"
new_to_old="$3"
shift 3

/cygdrive/c/progra~2/7-Zip/7z.exe e -y -oold-juju C:\\users\\Administrator\\old-juju\\win\\juju-$old_juju_version-win.zip
/cygdrive/c/progra~2/innoextract/innoextract.exe -e C:\\users\\Administrator\\candidate\\win\\juju-setup-$candidate_version.exe -d candidate

if [ "$new_to_old" = "true" ]; then
  echo "Server:" `candidate/app/juju.exe --version`
  echo "Client:" `old-juju/juju.exe --version`
  server=candidate\\app\\juju.exe
  client=old-juju\\juju.exe
else
  echo "Server:" `old-juju/juju.exe --version`
  echo "Client:" `candidate/app/juju.exe --version`
  client=candidate\\app\\juju.exe
  server=old-juju\\juju.exe
fi

mkdir logs
juju destroy-environment --force -y compatibility-control || true
python C:\\users\\Administrator\\juju-ci-tools\\assess_heterogeneous_control.py \
  $server $client test-win-client-server compatibility-control logs "$@"