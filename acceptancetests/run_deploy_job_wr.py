#!/usr/bin/env python
import json
import os
from os.path import join
import subprocess
import sys
from tempfile import NamedTemporaryFile


def main():
    revision_build = os.environ['revision_build']
    job_name = os.environ['JOB_NAME']
    build_number = os.environ['BUILD_NUMBER']
    prefix = 'juju-ci/products/version-{}/{}/build-{}'.format(
        revision_build, job_name, build_number)
    s3_config = join(os.environ['HOME'], 'cloud-city/juju-qa.s3cfg')
    command = [
        '$HOME/juju-ci-tools/run-deploy-job-remote.bash',
        revision_build,
        job_name,
        ]
    command.extend(sys.argv[2:])
    with NamedTemporaryFile() as config_file:
        json.dump({
            'command': command, 'install': {},
            'artifacts': {'artifacts': [
                'artifacts/machine*/*log*',
                'artifacts/*.jenv',
                'artifacts/cache.yaml',
                'artifacts/*.json',
                ]},
            'bucket': 'juju-qa-data',
            }, config_file)
        config_file.flush()
        subprocess.check_call([
            'workspace-run', config_file.name, sys.argv[1], prefix,
            '--s3-config', s3_config, '-v',
            ])


if __name__ == '__main__':
    main()
