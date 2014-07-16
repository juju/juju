#!/usr/bin/env python

"""Copy build artifacts to the S3 bucket 'juju-qa-data'.

Intended to be used in the Jenkins job "revision-results".

Required environment variables:

    BUILD_NUMBER - The build number of the revision-resulst job (set by
        Jenkins)
    S3CFG - Path to the config file for s3cmd. Default:
        ~/cloud-city/juju-qa.s3cfg
    build_number - (a Jenkins build parameter) The main buld number
    build_info - (a Jenkins build parameter) A build summary that is
        stored in the S3 bucket.
    job_builds - A YAML representation of a sequence
        (job_name, build_number),... where job_name is the name of a
        build or test job, and build_number is the "interesting" build
        number of this job. All artifacts for these builds will be copied
        to the S3 bucket.
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


def copy_to_s3(source_path, dest_url):
    subprocess.check_call([
        's3cmd', '-c', os.getenv('S3CFG'), '--no-progress', 'put',
        source_path, dest_url])


def copy_job_artifacts(juju_ci, main_build_number, job_name, job_build_number,
                       work_dir):
    build_info = juju_ci.get_build_info(job_name, job_build_number)
    for artifact in build_info['artifacts']:
        filename = artifact['fileName']
        if filename == 'empty':
            continue
        relative_path = artifact['relativePath']
        source_url = '%s/artifact/%s' % (
            build_info['url'], relative_path)
        dest_url = '%s/juju-ci/products/build-%s/%s/%s' % (
            ARCHIVE_BUCKET_URL, main_build_number, job_name, relative_path)
        local_path = os.path.join(work_dir, filename)
        subprocess.check_call(['wget', '-q', '-O', local_path, source_url])
        copy_to_s3(local_path, dest_url)


def save_build_status(build_number, work_dir):
    result_path = os.path.join(work_dir, 'result.yaml')
    with open(result_path, 'w') as results:
        build_status = os.getenv('build_status')
        results.write(build_status)
    s3_url = '%s/juju-ci/products/build-%s/result.yaml' % (
        ARCHIVE_BUCKET_URL, build_number)
    copy_to_s3(result_path, s3_url)


if __name__ == '__main__':
    juju_ci = Jenkins(JENKINS_URL)
    main_build_number = int(os.getenv('build_number'))

    with temp_dir() as work_dir:
        save_build_status(main_build_number, work_dir)
        job_builds = yaml.load(StringIO(os.getenv('job_builds')))
        for job_name, job_build_number in job_builds:
            copy_job_artifacts(
                juju_ci, main_build_number, job_name, job_build_number, work_dir)
