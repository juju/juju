#!/usr/bin/env python

"""Copy build artifacts to the S3 bucket 'juju-qa-data'.

Required environment variables:

    BUILD_NUMBER - set by Jenkins
    S3CFG - Path to the config file for s3cmd. Default:
            ~/cloud-city/juju-qa.s3cfg
"""

from __future__ import print_function
from jenkins import Jenkins
import os
import re
import subprocess
import sys
from utility import temp_dir


ARCHIVE_BUCKET_URL = 's3://juju-qa-data'
JENKINS_URL = 'http://juju-ci.vapour.ws:8080'
JOBS = {
    'build-revision': (
        (re.compile(r'^juju-core_.*\.tar\.gz$'), 'Juju core tarball'),
        ),
    'win-client-build-installer': (
        (re.compile(r'^juju-setup-.*exe$'), 'Windows installer'),
        ),
    }


def copy_job_artifacts(job, artifact_matchers, juju_ci, work_dir):
    job_info = juju_ci.get_job_info(job)
    last_successful_build = job_info['lastSuccessfulBuild']
    build_info = juju_ci.get_build_info(job, last_successful_build['number'])
    for matcher, name in artifact_matchers:
        source_url = None
        for artifact in build_info['artifacts']:
            if matcher.search(artifact['fileName']) is not None:
                source_url = '%s/artifact/%s' % (
                    build_info['url'], artifact['fileName'])
                dest_url = '%s/juju-ci/products/build-%s/%s' % (
                    ARCHIVE_BUCKET_URL, os.getenv('BUILD_NUMBER'),
                    artifact['fileName'])
                local_path = os.path.join(work_dir, artifact['fileName'])
                break
        if source_url is None:
            print(
                "Cannot find %s in artifacts of %s" % (name, job),
                file=sys.stderr)
            continue

        subprocess.check_call(['wget', '-q', '-O', local_path, source_url])
        subprocess.check_call([
            's3cmd', '-c', os.getenv('S3CFG'), '--no-progress', 'put',
            local_path, dest_url])


juju_ci = Jenkins(JENKINS_URL)

with temp_dir() as work_dir:
    for job, artifact_matchers in JOBS.items():
        copy_job_artifacts(job, artifact_matchers, juju_ci, work_dir)
