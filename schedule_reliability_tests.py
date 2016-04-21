#!/usr/bin/env python
from argparse import ArgumentParser

from jujuci import (
    add_credential_args,
    get_credentials,
    )
from utility import (
    find_latest_branch_candidates,
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
    parser.add_argument('--suite', help='Test suite to run', default=[],
                        choices=suites.keys(), action='append')
    parser.add_argument('jobs', nargs='*', metavar='job',
                        help='Jobs to schedule builds for.')
    add_credential_args(parser)
    result = parser.parse_args(argv)
    if result.jobs == []:
        result.jobs = None
    credentials = get_credentials(result)
    return result, credentials


def build_job(credentials, root, job_name, candidates, suite):
    parameters = {'suite': ','.join(suite), 'attempts': '10'}
    jenkins = Jenkins('http://localhost:8080', credentials.user,
                      credentials.password)
    for candidate, revision_build in candidates:
        call_parameters = {
            'revision_build': '{:d}'.format(revision_build),
            }
        call_parameters.update(parameters)
        token = get_auth_token(root, job_name)
        jenkins.build_job(job_name, call_parameters, token=token)


def main(argv=None):
    args, credentials = parse_args(argv)
    suite = args.suite
    if suite == []:
        suite = [FULL]
    candidates = find_latest_branch_candidates(args.root_dir)[:3]
    jobs = args.jobs
    if jobs is None:
        jobs = ['industrial-test', 'industrial-test-aws',
                'industrial-test-joyent']
    for job in jobs:
        build_job(credentials, args.root_dir, job, candidates, suite)


if __name__ == '__main__':
    main()
