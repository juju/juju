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


def get_args():
    parser = ArgumentParser()
    parser.add_argument(
        'root_dir', help='Directory containing releases and candidates dir')
    return parser.parse_args()


def find_candidates(root_dir):
    candidates_path = os.path.join(root_dir, 'candidate')
    a_week_ago = time() - timedelta(days=7).total_seconds()
    for candidate_dir in os.listdir(candidates_path):
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


def build_job(root, job_name, candidates):
    parameters = {'suite': 'density'}
    jenkins = Jenkins('http://localhost:8080')
    for candidate in candidates:
        parameters['new_juju_dir'] = candidate
        token = get_auth_token(root, job_name)
        jenkins.build_job(job_name, parameters, token=token)


def main():
    args = get_args()
    candidates = list(find_candidates(args.root_dir))
    for job in ['industrial-test', 'industrial-test-aws',
                'industrial-test-joyent']:
        build_job(args.root_dir, job, candidates)


if __name__ == '__main__':
    main()
