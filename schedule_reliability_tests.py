#!/usr/bin/env python
from argparse import ArgumentParser
from datetime import (
    timedelta,
    )
import errno
import os
from time import time
from utility import get_auth_token

from jenkins import Jenkins

from industrial_test import (
    FULL,
    suites,
    )


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument(
        'root_dir', help='Directory containing releases and candidates dir')
    parser.add_argument('--suite', help='Test suite to run', default=FULL,
                        choices=suites.keys())
    return parser.parse_args(argv)


def find_candidates(root_dir):
    candidates_path = os.path.join(root_dir, 'candidate')
    a_week_ago = time() - timedelta(days=7).total_seconds()
    for candidate_dir in os.listdir(candidates_path):
        if candidate_dir.endswith('-artifacts'):
            continue
        candidate_path = os.path.join(candidates_path, candidate_dir)
        buildvars = os.path.join(candidate_path, 'buildvars.json')
        try:
            stat = os.stat(buildvars)
        except OSError as e:
            if e.errno in (errno.ENOENT, errno.ENOTDIR):
                continue
            raise
        if stat.st_mtime < a_week_ago:
            continue
        yield candidate_path


def build_job(root, job_name, candidates, suite):
    parameters = {'suite': suite, 'attempts': '10'}
    jenkins = Jenkins('http://localhost:8080')
    for candidate in candidates:
        call_parameters = {'new_juju_dir': candidate}
        call_parameters.update(parameters)
        token = get_auth_token(root, job_name)
        jenkins.build_job(job_name, call_parameters, token=token)


def main():
    args = parse_args()
    candidates = list(find_candidates(args.root_dir))
    for job in ['industrial-test', 'industrial-test-aws',
                'industrial-test-joyent']:
        build_job(args.root_dir, job, candidates, args.suite)


if __name__ == '__main__':
    main()
