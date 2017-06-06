#!/usr/bin/env python
from argparse import ArgumentParser
import subprocess

from jenkins import Jenkins

from jujuci import add_credential_args
from utility import (
    find_candidates,
    )


def start_job(root, job, juju_bin, user, password, number):
    """Use Jenkins API to start a job."""
    jenkins = Jenkins('http://juju-ci.vapour.ws:8080', user, password)
    job_params = {'juju_bin': juju_bin, 'sequence_number': number}
    jenkins.build_job(job, job_params)


def parse_args(argv=None):
    parser = ArgumentParser()
    add_credential_args(parser)
    parser.add_argument('job', help='Jenkins job to run')
    parser.add_argument('root_dir', help='Jenkins home directory.')
    parser.add_argument('count', default=10, type=int,
                        help='The number of Jenkins jobs to run.')
    parser.add_argument('--all', action='store_true', default=False,
                        help='Schedule all candidates vs. weekly.')
    args = parser.parse_args(argv)
    if not args.user:
        parser.error("The Jenkins user must be given with --user or be set in "
                     "the environment as JENKINS_USER.")
    if not args.password:
        parser.error("The Jenkins password must be given with --password or "
                     "be set in the environment as JENKINS_PASSWORD.")
    return args


def main(argv=None):
    args = parse_args(argv)
    for candidate_path in find_candidates(args.root_dir, args.all):
        juju_bin = subprocess.check_output(
            ['find', candidate_path, '-name', 'juju']).strip()
        for i in range(args.count):
            start_job(args.root_dir, args.job, juju_bin, args.user,
                      args.password, i)


if __name__ == '__main__':
    main()
