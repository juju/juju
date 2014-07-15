#!/usr/bin/env python

"""Copy build artifacts to the S3 bucket 'juju-qa-data'.

Required environment variables:

    BUILD_NUMBER - The build number of the revision-resulst job (set by
        Jenkins)
    S3CFG - Path to the config file for s3cmd. Default:
        ~/cloud-city/juju-qa.s3cfg
"""

from __future__ import print_function
from cStringIO import StringIO
from jenkins import Jenkins
import os
import subprocess
from utility import temp_dir
import yaml


ARCHIVE_BUCKET_URL = 's3://juju-qa-data'
JENKINS_URL = 'http://juju-ci.vapour.ws:8080'
# Associate a job key as used by ci-director with a Jenkins job name.
JOBS = (
    ('build', 'build-revision'),
    ('build-windows-installer', 'win-client-build-installer'),
    ('deploy-via-windows', 'win-client-deploy'),
    ('publication', 'publish-revision'),
    )


def copy_to_s3(source_path, dest_url):
    subprocess.check_call([
        's3cmd', '-c', os.getenv('S3CFG'), '--no-progress', 'put',
        source_path, dest_url])


def copy_job_artifacts(juju_ci, build_number, build_summary, job_name,
                       work_dir):
    if 'last_build' not in build_summary:
        return
    build_info = juju_ci.get_build_info(job_name, build_summary['last_build'])
    for artifact in build_info['artifacts']:
        filename = artifact['fileName']
        if filename == 'empty':
            continue
        relative_path = artifact['relativePath']
        source_url = '%s/artifact/%s' % (
            build_info['url'], relative_path)
        dest_url = '%s/juju-ci/products/build-%s/%s/%s' % (
            ARCHIVE_BUCKET_URL, build_number, job_name, relative_path)
        local_path = os.path.join(work_dir, filename)
        subprocess.check_call(['wget', '-q', '-O', local_path, source_url])
        copy_to_s3(local_path, dest_url)


def save_build_status(build_status, build_number, work_dir):
    result_path = os.path.join(work_dir, 'result.yaml')
    with open(result_path, 'w') as results:
        results.write(build_status)
    s3_url = '%s/juju-ci/products/build-%s/result.yaml' % (
        ARCHIVE_BUCKET_URL, build_number)
    copy_to_s3(result_path, s3_url)


if __name__ == '__main__':
    juju_ci = Jenkins(JENKINS_URL)
    build_number = int(os.getenv('build_number'))

    with temp_dir() as work_dir:
        build_status = os.getenv('build_status')
        save_build_status(build_status, build_number, work_dir)
        build_status = yaml.load(StringIO(build_status))
        for job_key, job_name in JOBS:
            copy_job_artifacts(
                juju_ci, build_number, build_status[job_key], job_name,
                work_dir)
        for test_name, summary in build_status['functional-tests'].items():
            copy_job_artifacts(
                juju_ci, build_number, summary, test_name, work_dir)
        for test_name, summary in build_status['tests'].items():
            copy_job_artifacts(
                juju_ci, build_number, summary, test_name, work_dir)
