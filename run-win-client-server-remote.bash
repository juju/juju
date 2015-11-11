# This script is intended to run on Windows. It extracts compressed Juju binary
# in a ZIP archive and run the client-server test.
set -eux

candidate_version="$1"
old_juju_version="$2"
new_to_old="$3"
log_dir="$4"
shift 4

if [ -f $HOME/old-juju/win/juju-$candidate_version-win.zip ]; then
    /cygdrive/c/progra~2/7-Zip/7z.exe e -y -ocandidate C:\\users\\Administrator\\old-juju\\win\\juju-$candidate_version-win.zip
   candidate_juju=candidate\\juju.exe
else
   /cygdrive/c/progra~2/innoextract/innoextract.exe -e C:\\users\\Administrator\\candidate\\win\\juju-setup-$candidate_version.exe -d candidate
   candidate_juju=candidate\\app\\juju.exe
fi
/cygdrive/c/progra~2/7-Zip/7z.exe e -y -oold-juju C:\\users\\Administrator\\old-juju\\win\\juju-$old_juju_version-win.zip

if [ "$new_to_old" = "true" ]; then
  server=$candidate_juju
  client=old-juju\\juju.exe
else
  client=$candidate_juju
  server=old-juju\\juju.exe
fi

echo "Server:" `$server --version`
echo "Client:" `$client --version`

mkdir $log_dir
juju destroy-environment --force -y compatibility-control-win || true
python C:\\users\\Administrator\\juju-ci-tools\\assess_heterogeneous_control.py \
  $server $client test-win-client-server compatibility-control-win $log_dir "$@"

