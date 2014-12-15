#!/usr/bin/env python
from argparse import ArgumentParser
from utility import (
    find_candidates,
    get_auth_token,
    )

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
    parser.add_argument('jobs', nargs='*', metavar='job',
                        help='Jobs to schedule builds for.')
    result = parser.parse_args(argv)
    if result.jobs == []:
        result.jobs = None
    return result


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
    jobs = args.jobs
    if jobs is None:
        jobs = ['industrial-test', 'industrial-test-aws',
                'industrial-test-joyent']
    for job in jobs:
        build_job(args.root_dir, job, candidates, args.suite)


if __name__ == '__main__':
    main()
