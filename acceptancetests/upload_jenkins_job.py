#!/usr/bin/env python

from __future__ import print_function

from argparse import ArgumentParser
import json
from mimetypes import MimeTypes
import os
import sys
from time import sleep
import urlparse

from boto.s3.connection import S3Connection
import requests
from requests.auth import HTTPBasicAuth

from jujuci import (
    get_build_data,
    add_credential_args,
    get_credentials,
    get_job_data,
    JENKINS_URL,
)
from s3ci import get_s3_credentials
from utility import until_timeout


__metaclass__ = type


CONSOLE_TEXT = 'consoleText'
RESULT_RESULTS = 'result-results.json'


class JenkinsBuild:
    """
    Retrieves Jenkins build information
    """

    def __init__(self, credentials, job_name, jenkins_url, build_info):
        """
        :param credentials: Jenkins credentials
        :param job_name:  Jenkins job name
        :param jenkins_url: Jenkins server URL
        :param build_info: Jenkins build info
        :return: None
        """
        self.credentials = credentials
        self.jenkins_url = jenkins_url
        self.job_name = job_name
        self.build_info = build_info

    @classmethod
    def factory(cls, credentials, job_name, build_number=None, url=None):
        """
        :param credentials: Jenkins credentials
        :param job_name: Jenkins job name
        :param build_number: Jenkins build number
        :param url: Jenkins url
        :return:
        """
        url = url or JENKINS_URL
        build_info = (get_build_data(url, credentials, job_name, build_number)
                      if build_number else None)
        return cls(credentials, job_name, url, build_info)

    def get_build_info(self, build_number=None):
        """
        Gets build info from the Jenkins server
        :rtype: dict
        """
        build_number = build_number or self.get_build_number()
        self.build_info = get_build_data(
            self.jenkins_url, self.credentials, self.job_name, build_number)
        return self.build_info

    def is_build_completed(self):
        """Check if the build is completed and return boolean."""
        build_info = self.get_build_info()
        return not build_info['building']

    @property
    def result(self):
        """
        Returns the test result string
        :return: SUCCESS, FAILURE, ABORTED, NOT_BUILT, SUCCESS, UNSTABLE ...
        :rtype: str
        """
        return self.build_info.get('result')

    def get_console_text(self):
        """
        Return Jenkins build console log
        :rtype: str
        """
        log_url = urlparse.urljoin(self.build_info['url'], CONSOLE_TEXT)
        return requests.get(
            log_url, auth=HTTPBasicAuth(
                self.credentials.user, self.credentials.password)).text

    def get_last_completed_build_number(self):
        """
        Returns latest Jenkins build number
        :rtype: int
        """
        job_info = get_job_data(
            self.jenkins_url, self.credentials, self.job_name)
        return job_info['lastCompletedBuild']['number']

    def artifacts(self):
        """
        Returns the filename and the content of artifacts
        :return: filename and artifacts content
        :rtype: tuple
        """
        relative_paths = [(x['relativePath'], x['fileName']) for x in
                          self.build_info['artifacts']]
        auth = HTTPBasicAuth(self.credentials.user, self.credentials.password)
        for path, filename in relative_paths:
            url = self._get_artifact_url(path)
            content = requests.get(url, auth=auth).content
            yield filename, content

    def _get_artifact_url(self, relative_path):
        """
        :return: List of artifact URLs
        :rtype: list
        """
        return self.build_info['url'] + 'artifact/' + relative_path

    def get_build_number(self):
        return self.build_info.get('number')

    def set_build_number(self, build_number):
        self.get_build_info(build_number)


class S3:
    """
    Used to store an object in S3
    """

    def __init__(self, directory, access_key, secret_key, conn, bucket):
        self.dir = directory
        self.access_key = access_key
        self.secret_key = secret_key
        self.conn = conn
        self.bucket = bucket

    @classmethod
    def factory(cls, bucket, directory):
        access_key, secret_key = get_s3_access()
        conn = S3Connection(access_key, secret_key)
        bucket = conn.get_bucket(bucket)
        return cls(directory, access_key, secret_key, conn, bucket)

    def store(self, filename, data, headers=None):
        """
        Stores an object in S3.
        :param filename: filename of the object
        :param data: The content to be stored in S3
        :rtype: bool
        """
        if not data:
            return False
        path = os.path.join(self.dir, filename)
        key = self.bucket.new_key(path)
        # This will store the data.
        key.set_contents_from_string(data, headers=headers)
        return True


class S3Uploader:
    """
    Uploads the result of a Jenkins job to S3.
    """

    def __init__(self, s3, jenkins_build, unique_id=None, no_prefixes=False,
                 artifact_file_ext=None):
        self.s3 = s3
        self.jenkins_build = jenkins_build
        self.unique_id = unique_id
        self.no_prefixes = no_prefixes
        self.artifact_file_ext = artifact_file_ext

    @classmethod
    def factory(cls, credentials, jenkins_job, build_number, bucket,
                directory, unique_id=None, no_prefixes=False,
                artifact_file_ext=None):
        """
        Creates S3Uploader.
        :param credentials: Jenkins credential
        :param jenkins_job: Jenkins job name
        :param build_number: Jenkins build number
        :param bucket: S3 bucket name
        :param directory: S3 directory name
        :param artifact_file_ext: List of artifact file extentions. If set,
        only artifact with these ejections will be uploaded.
        :rtype: S3Uploader
        """
        s3 = S3.factory(bucket, directory)
        build_number = int(build_number) if build_number else build_number
        jenkins_build = JenkinsBuild.factory(
            credentials=credentials, job_name=jenkins_job,
            build_number=build_number)
        return cls(s3, jenkins_build,
                   unique_id=unique_id, no_prefixes=no_prefixes,
                   artifact_file_ext=artifact_file_ext)

    def upload(self):
        """Uploads Jenkins job results, console logs and artifacts to S3.

        :return: None
        """
        self.upload_test_results()
        self.upload_console_log()
        self.upload_artifacts()

    def upload_by_build_number(self, build_number=None, pause_time=120,
                               timeout=600):
        """
        Upload build_number's test result.

        :param build_number:
        :param pause_time: Pause time in seconds between polling.
        :param timeout: Timeout in seconds.
        :return: None
        """
        build_number = build_number or os.getenv('BUILD_NUMBER')
        if not build_number:
            raise ValueError('Build number is not set')
        self.jenkins_build.set_build_number(build_number)
        for _ in until_timeout(timeout):
            if self.jenkins_build.is_build_completed():
                break
            sleep(pause_time)
        else:
            raise Exception("Build fails to complete: {}".format(build_number))
        self.upload()

    def upload_all_test_results(self):
        """
        Uploads all the test results to S3. It starts with the build_number 1
        :return: None
        """
        latest_build_num = self.jenkins_build.get_last_completed_build_number()
        for build_number in range(1, latest_build_num + 1):
            self.jenkins_build.set_build_number(build_number)
            self.upload()

    def upload_last_completed_test_result(self):
        """Upload the latest test result to S3."""
        latest_build_num = self.jenkins_build.get_last_completed_build_number()
        self.jenkins_build.set_build_number(latest_build_num)
        self.upload()

    def upload_test_results(self):
        filename = self._create_filename(RESULT_RESULTS)
        headers = {"Content-Type": "application/json"}
        build_info = self.jenkins_build.get_build_info()
        if self.unique_id:
            build_info['origin_number'] = int(build_info['number'])
            build_info['number'] = int(self.unique_id)
        self.s3.store(
            filename, json.dumps(build_info, indent=4),
            headers=headers)

    def upload_console_log(self):
        filename = self._create_filename(CONSOLE_TEXT)
        headers = {"Content-Type": "text/plain; charset=utf8"}
        self.s3.store(
            filename, self.jenkins_build.get_console_text(), headers=headers)

    @staticmethod
    def make_headers(filename):
        mime = MimeTypes()
        mime.add_type('text/plain', '.log')
        mime.add_type('text/x-yaml', '.yaml')
        content_type, encoding = mime.guess_type(filename)
        headers = {"Content-Type": "application/octet-stream"}
        if content_type:
            headers['Content-Type'] = content_type
        if encoding:
            headers['Content-Encoding'] = encoding
        return headers

    def upload_artifacts(self):
        for filename, content in self.jenkins_build.artifacts():
            if self.artifact_file_ext:
                if os.path.splitext(filename)[1] not in self.artifact_file_ext:
                    continue
            filename = self._create_filename(filename)
            headers = self.make_headers(filename)
            self.s3.store(filename, content, headers=headers)

    def _create_filename(self, filename):
        """
        Creates filename based on the combination of the job ID and the
        filename
        :return: Filename
        :rtype: str
        """
        if self.no_prefixes:
            return filename
        # Rules for dirs with files from several job builds.
        if filename == CONSOLE_TEXT:
            filename = 'console-consoleText.txt'
        elif filename != RESULT_RESULTS:
            filename = 'log-' + filename
        if self.unique_id:
            return "{}-{}".format(self.unique_id, filename)
        return str(self.jenkins_build.get_build_number()) + '-' + filename


def get_s3_access():
    """
    Return S3 access and secret keys
    """
    s3cfg_path = os.path.join(
        os.getenv('HOME'), 'cloud-city/juju-qa.s3cfg')
    return get_s3_credentials(s3cfg_path)


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('jenkins_job', help="Jenkins job name.")
    parser.add_argument(
        'build_number', default=None,
        help="Build to upload, can be a number, 'all' or 'latest'.")
    parser.add_argument('s3_bucket', help="S3 bucket name to store files.")
    parser.add_argument(
        's3_directory',
        help="Directory under the bucket name to store files.")
    parser.add_argument(
        '--unique-id',
        help='Unique ID to be used to generate file names. If this is not '
             'set, the parent build number will be used as a unique ID.')
    parser.add_argument(
        '--no-prefixes', action='store_true', default=False,
        help='Do not add prefixes to file names; the s3_directory is unique.')
    parser.add_argument(
        '--artifact-file-ext', nargs='+',
        help='Artifacts include file extentions. If set, only files with '
             'these extentions will be uploaded.')
    add_credential_args(parser)
    args = parser.parse_args(argv)
    args.all = False
    args.latest = False
    if args.build_number == 'all':
        args.all = True
        args.build_number = None
    if args.build_number == 'latest':
        args.latest = True
        args.build_number = None
    if args.build_number:
        args.build_number = int(args.build_number)
    return args


def main(argv=None):
    args = get_args(argv)
    cred = get_credentials(args)
    uploader = S3Uploader.factory(
        cred, args.jenkins_job, args.build_number, args.s3_bucket,
        args.s3_directory, unique_id=args.unique_id,
        no_prefixes=args.no_prefixes, artifact_file_ext=args.artifact_file_ext)
    if args.build_number:
        print('Uploading build number {:d}.'.format(args.build_number))
        uploader.upload()
    elif args.all:
        print('Uploading all test results.')
        uploader.upload_all_test_results()
    elif args.latest:
        print('Uploading the latest test result.')
        print('WARNING: latest can be a moving target.')
        uploader.upload_last_completed_test_result()


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
