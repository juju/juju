#!/usr/bin/env python
import json
import os
import subprocess
import sys
from tempfile import NamedTemporaryFile

def main():
    command = [
        '$HOME/juju-ci-tools/run-deploy-job-remote.bash',
        os.environ['revision_build'],
        os.environ['JOB_NAME'],
        ]
    command.extend(sys.argv[2:])
    with NamedTemporaryFile() as config_file:
        json.dump({'command': command, 'install': {}}, config_file)
        config_file.flush()
        subprocess.check_call(['workspace-run', config_file.name, sys.argv[1]])

if __name__ == '__main__':
    main()
