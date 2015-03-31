#!/usr/bin/env python
from __future__ import print_function

import os
import yaml
import requests
import urlparse
from jenkins import Jenkins
from requests.auth import HTTPBasicAuth
from boto.s3.connection import S3Connection
from ConfigParser import ConfigParser
import json


class JenkinsJob(object):
    def __init__(self, job_name, job_id=None, url=None, username=None,
                 password=None):
        """
        :param job_name: Jenkins job name
        :param job_id:  Jenkins job id. If not set, this will be set to
            the latest job id
        :param url: URL to the Jenkins server
        :param username: Jenkins username
        :param password: Jenkins password
        :return None:
        """
        self.username = username
        self.password = password
        self.url = url
        self.jenkins = self._create()
        self.job_name = job_name
        self.job_id = job_id or self.latest_build_number
        self.build_info = self.jenkins.get_build_info(
            self.job_name, self.job_id)

    @property
    def job_id(self):
        return self._job_id

    @job_id.setter
    def job_id(self, value):
        self._job_id = value
        self.build_info = self.jenkins.get_build_info(
            self.job_name, self.job_id)

    @property
    def result(self):
        """
        Returns Jenkins build result
        :return: SUCCESS, FAILURE, ABORTED, NOT_BUILT, SUCCESS, UNSTABLE ...
        :rtype: str
        """
        return self.build_info.get('result')

    def _create(self):
        """
        Creates Jenkins object
        :return: Jenkins object
        :rtype: Jenkins
        """
        if self.url and self.username and self.password:
            return Jenkins(self.url, self.username, self.password)

        # TODO: review this code...is this the right way to get username/pass?
        cloud_path = os.path.dirname(os.path.realpath(__file__))
        config_path = os.path.join(cloud_path, '../cloud-city/jenkins.yaml')
        with open(config_path, 'r') as outfile:
            config = yaml.load(outfile)

        self.url = self.url or config['url']
        self.username = self.username or config['username']
        self.password = self.password or config['password']

        return Jenkins(self.url, self.username, self.password)

    @property
    def parameters(self):
        """
        Return Jenkins build parameters
        :return: Build Parameters
        :rtype: dict
        """
        parameters = {}
        for action in self.build_info.get('actions', []):
            parameter_list = action.get('parameters', [])
            parameters.update((p['name'], p['value']) for p in parameter_list)
        return parameters

    @property
    def console_text(self):
        """
        Return Jenkins' build console text
        :return:
        :rtype: str
        """
        log_url = urlparse.urljoin(self.build_info['url'], 'consoleText')
        return requests.get(
            log_url, auth=HTTPBasicAuth(self.username, self.password)).text

    @property
    def latest_build_number(self):
        """
        Latest Jenkins build number
        :return:
        :rtype: int
        """
        job_info = self.jenkins.get_job_info(self.job_name)
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
        for path, filename in relative_paths:
            url = self._get_artifact_url(path)
            content = requests.get(
                url, auth=HTTPBasicAuth(self.username, self.password)).content
            yield filename, content

    def _get_artifact_url(self, relative_path):
        """
        :return: List of artifact URLs
        :rtype: list
        """
        return self.build_info['url'] + 'artifact/' + relative_path


class S3:

    def __init__(self):
        s3cfg_path = os.path.join(
            os.environ['HOME'], 'cloud-city/juju-qa.s3cfg')
        c = ConfigParser()
        c.readfp(open(s3cfg_path))
        self.folder = '/comp-test'
        self.access_key = c.get('default', 'access_key')
        self.secret_key = c.get('default', 'secret_key')
        conn = S3Connection(self.access_key, self.secret_key)
        self.bucket = conn.get_bucket('juju-qa-data')

    def store(self, filename, data, headers=None):
        """
        Stores an object in S3
        :param filename: filename of the object
        :param data: The content to be stored in S3
        :return None:
        """
        if not data:
            return False
        path = os.path.join(self.folder, filename)
        key = self.bucket.new_key(path)
        key.set_contents_from_string(data, headers=headers)
        return True


class HUploader(JenkinsJob):
    """
    Uploads the result of Heterogeneous Control test to S3
    """

    def __init__(self, job_id=None):
        """
        :param job_id: If job_id is not set, it will be set to the latest
        job id number
        :return:
        """
        self.s3_path = 's3://juju-qa-data/comp-test/'
        super(HUploader, self).__init__(job_name='compatibility-control',
                                        job_id=job_id)

    def upload(self):
        """
        Uploads the Heterogeneous Control test results to S3. Uploads the
        test results, console logs and artifacts.

        :return: None
        """
        self.upload_test_results()
        self.upload_console_log()
        self.upload_artifacts()

    def upload_all_test_results(self):
        """
        Uploads all the test results to S3. It starts with the job_id set to 1
        to the latest job_id

        :return: None
        """
        latest_job_id = self.latest_build_number
        for job_id in xrange(1, latest_job_id + 1):
            self.job_id = job_id
            self.upload()

    def upload_test_results(self):
        """
        :return: None
        """
        s3 = S3()
        filename = self._create_filename('result-results.json')
        headers = {"Content-Type": "application/json"}
        s3.store(filename, data=json.dumps(self.build_info), headers=headers)

    def upload_console_log(self):
        """
        :return: None
        """
        s3 = S3()
        filename = self._create_filename('console-consoleText.txt')
        headers = {"Content-Type": "text/plain"}
        s3.store(filename, self.console_text, headers=headers)

    def upload_artifacts(self):
        """
        :return: None
        """
        s3 = S3()
        headers = {"Content-Type": "application/octet-stream"}
        for filename, content in self.artifacts():
            filename = self._create_filename('log-' + filename)
            s3.store(filename, content, headers)

    def _create_filename(self, filename):
        """
        Creates filename based on the combination of the job ID and the
        filename

        :return: Filename
        :rtype: str
        """
        if not self.job_id:
            raise ValueError('Job ID must be set to create filename')
        return str(self.job_id) + '-' + filename


#Todo: for testing only
u = HUploader()
u.upload_all_test_results()
