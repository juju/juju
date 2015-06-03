#!/usr/bin/env python

from __future__ import print_function

from argparse import ArgumentParser
from ConfigParser import ConfigParser
import json
import os
import sys
import urlparse

from boto.s3.connection import S3Connection
from jenkins import Jenkins
import requests
from requests.auth import HTTPBasicAuth

from jujuci import(
    get_build_data,
    add_credential_args,
    get_credentials,
    get_job_data,
    JENKINS_URL,
)


__metaclass__ = type


class JenkinsBuild:
    """
    Retrieves Jenkins build information
    """

    def __init__(self, credentials, job_name, jenkins, build_info):
        """
        :param credentials: Jenkins credentials
        :param job_name:  Jenkins job name
        :param jenkins: Jenkins object
        :param build_info: Jenkins build info
        :return: None
        """
        self.credentials = credentials
        self.jenkins = jenkins
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
        jenkins = Jenkins(url, *credentials)
        build_info = (jenkins.get_build_info(job_name, build_number)
                      if build_number else None)
        return cls(credentials, job_name, jenkins, build_info)

    def get_build_info(self, build_number=None):
        """
        Gets build info from the Jenkins server
        :rtype: dict
        """
        build_number = build_number or self.get_build_number()
        return get_build_data(
            self.jenkins.server, self.credentials, self.job_name, build_number)

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
        log_url = urlparse.urljoin(self.build_info['url'], 'consoleText')
        return requests.get(
            log_url, auth=HTTPBasicAuth(
                self.credentials.user, self.credentials.password)).text

    def get_latest_build_number(self):
        """
        Returns latest Jenkins build number
        :rtype: int
        """
        job_info = get_job_data(
            self.jenkins.server, self.credentials, self.job_name)
        if not job_info or not job_info.get('lastBuild'):
            return None
        return job_info.get('lastBuild').get('number')

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
        self.build_info = self.get_build_info(build_number)


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
    def factory(cls):
        directory = '/comp-test'
        access_key, secret_key = get_s3_access()
        conn = S3Connection(access_key, secret_key)
        bucket = conn.get_bucket('juju-qa-data')
        return cls(directory, access_key, secret_key, conn, bucket)

    def store(self, filename, data, headers=None):
        """
        Stores an object in S3
        :param filename: filename of the object
        :param data: The content to be stored in S3
        :rtype: bool
        """
        if not data:
            return False
        path = os.path.join(self.dir, filename)
        key = self.bucket.new_key(path)
        # This will store the data in s3://juju-qa-data/comp-test/
        key.set_contents_from_string(data, headers=headers)
        return True


class HUploader:
    """
    Uploads the result of Heterogeneous Control test to S3
    """

    def __init__(self, s3, jenkins_build):
        self.s3 = s3
        self.jenkins_build = jenkins_build

    @classmethod
    def factory(cls, credentials, build_number=None):
        """
        Creates HUploader.
        :param credentials: Jenkins credential
        :param build_number: Jenkins build number
        :rtype: HUploader
        """
        s3 = S3.factory()
        build_number = int(build_number) if build_number else build_number
        jenkins_build = JenkinsBuild.factory(
            credentials=credentials, job_name='compatibility-control',
            build_number=build_number)
        return cls(s3, jenkins_build)

    def upload(self):
        """
        Uploads the Heterogeneous Control test results to S3. Uploads the
        test results, console logs and artifacts.
        :return: None
        """
        self.upload_test_results()
        self.upload_console_log()
        self.upload_artifacts()

    def upload_by_env_build_number(self):
        """
        Uploads a test result by first getting the build number from  the
        environment variable
        :return: None
        """
        build_number = os.getenv('BUILD_NUMBER')
        if not build_number:
            raise ValueError('Build number is not set')
        self.jenkins_build.set_build_number(int(build_number))
        self.upload()

    def upload_all_test_results(self):
        """
        Uploads all the test results to S3. It starts with the build_number 1
        :return: None
        """
        latest_build_num = self.jenkins_build.get_latest_build_number()
        for build_number in range(1, latest_build_num + 1):
            self.jenkins_build.set_build_number(build_number)
            self.upload()

    def upload_test_results(self):
        filename = self._create_filename('result-results.json')
        headers = {"Content-Type": "application/json"}
        self.s3.store(filename, json.dumps(
            self.jenkins_build.get_build_info()), headers=headers)

    def upload_console_log(self):
        filename = self._create_filename('console-consoleText.txt')
        headers = {"Content-Type": "text/plain; charset=utf8"}
        self.s3.store(
            filename, self.jenkins_build.get_console_text(), headers=headers)

    def upload_artifacts(self):
        headers = {"Content-Type": "application/octet-stream"}
        for filename, content in self.jenkins_build.artifacts():
            filename = self._create_filename('log-' + filename)
            self.s3.store(filename, content, headers=headers)

    def _create_filename(self, filename):
        """
        Creates filename based on the combination of the job ID and the
        filename
        :return: Filename
        :rtype: str
        """
        return str(self.jenkins_build.get_build_number()) + '-' + filename


def get_s3_access():
    """
    Return S3 access and secret keys
    """
    s3cfg_path = os.path.join(
        os.getenv('HOME'), 'cloud-city/juju-qa.s3cfg')
    config = ConfigParser()
    config.read(s3cfg_path)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key


if __name__ == '__main__':
    parser = ArgumentParser()
    add_credential_args(parser)

    parser.add_argument(
        '-b', '--build_number', action='append',
        help="Specify build number to upload")
    parser.add_argument(
        '-a', '--all', action='store_true', default=False,
        help="Upload all test results")

    args = parser.parse_args(sys.argv[1:])
    cred = get_credentials(args)
    build_num = None

    if args.build_number:
        build_num = args.build_number[0]
        print('Uploading build number ' + build_num)
        u = HUploader.factory(credentials=cred, build_number=int(build_num))
        sys.exit(u.upload())
    elif args.all:
        print('Uploading all test results')
        u = HUploader.factory(credentials=cred)
        sys.exit(u.upload_all_test_results())
