#!/usr/bin/env python

"""Copy build artifacts to the S3 bucket 'juju-qa-data'.

Required environment variables:

    BUILD_NUMBER - The build number of the revision-resulst job (set by
        Jenkins)
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
BUILD_REVISION_JOB = 'build-revision'
REVISION_RESULTS_JOB = 'revision-results'
PARAM_REVISION_BUILD = 'revision_build'
JOBS = {
    BUILD_REVISION_JOB: (
        (re.compile(r'^juju-core_.*\.tar\.gz$'), 'Juju core tarball'),
        ),
    'win-client-build-installer': (
        (re.compile(r'^juju-setup-.*exe$'), 'Windows installer'),
        ),
    }


def get_build_parameter(build_info, name):
    for action in build_info.get('actions', []):
        for parameter in action.get('parameters', []):
            if parameter['name'] == name:
                return parameter['value']


def revision_build_from_revision_results(juju_ci):
    """Return the build revision from the parameters of the current build."""
    build_info = juju_ci.get_build_info(
        REVISION_RESULTS_JOB, int(os.getenv('BUILD_NUMBER')))
    revision_build = get_build_parameter(build_info, 'build_number')
    if revision_build is None:
        raise ValueError('Main build number not found in build parameters')
    return int(revision_build)


def job_build_number_for_revision_build(juju_ci, job_name, revision_build):
    """Find the build number for the given job where the parameter
    revision_build matches the passed value of revision_build.
    """
    # The job 'build_revison' itself does not have this parameter.
    # In this case, we know the required build number already: The
    # value of revision_build.
    if job_name == BUILD_REVISION_JOB:
        return revision_build
    job_info = juju_ci.get_job_info(job_name)
    for build_number in [build['number'] for build in job_info['builds']]:
        build_info = juju_ci.get_build_info(job_name, build_number)
        found_revision_build = get_build_parameter(
            build_info, PARAM_REVISION_BUILD)
        if (found_revision_build is not None and
            int(found_revision_build) == revision_build):
            return build_number
    print("Cannot find a build of %s for revision_build %s" %
          (job_name, revision_build), file=sys.stderr)
    return None


def copy_job_artifacts(job_name, revision_build, artifact_matchers, juju_ci,
                        work_dir):
    build_number = job_build_number_for_revision_build(
        juju_ci, job_name, revision_build)
    if build_number is None:
        return
    build_info = juju_ci.get_build_info(job_name, build_number)
    for matcher, name in artifact_matchers:
        source_url = None
        for artifact in build_info['artifacts']:
            filename = artifact['fileName']
            if matcher.search(filename) is not None:
                source_url = '%s/artifact/%s' % (build_info['url'], filename)
                dest_url = '%s/juju-ci/products/build-%s/%s' % (
                    ARCHIVE_BUCKET_URL, revision_build, filename)
                local_path = os.path.join(work_dir, filename)
                break
        if source_url is None:
            print(
                "Cannot find %s in artifacts of %s" % (name, job_name),
                file=sys.stderr)
            continue

        subprocess.check_call(['wget', '-q', '-O', local_path, source_url])
        subprocess.check_call([
            's3cmd', '-c', os.getenv('S3CFG'), '--no-progress', 'put',
            local_path, dest_url])

if __name__ == '__main__':
    juju_ci = Jenkins(JENKINS_URL)

    with temp_dir() as work_dir:
        revision_build = revision_build_from_revision_results(juju_ci)
        for job_name, artifact_matchers in JOBS.items():
            copy_job_artifacts(
                job_name, revision_build, artifact_matchers, juju_ci, work_dir)
