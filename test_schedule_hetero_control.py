from __future__ import print_function

__metaclass__ = type

import json
import os
from unittest import TestCase

from mock import patch

from jujuci import Credentials
from schedule_hetero_control import (
    build_jobs,
    calculate_jobs,
    get_args,
    get_candidate_version,
    )
from utility import temp_dir
from test_utility import write_config


class TestGetArgs(TestCase):

    def test_get_args_credentials(self):
        args, credentials = get_args(['root', '--user', 'jrandom',
                                      '--password', 'password'])
        self.assertEqual(args.user, 'jrandom')
        self.assertEqual(args.password, 'password')
        self.assertEqual(credentials, Credentials('jrandom', 'password'))


class TestBuildJobs(TestCase):

    def test_build_jobs_credentials(self):
        credentials = Credentials('jrandom', 'password1')
        with temp_dir() as root:
            write_config(root, 'compatibility-control', 'asdf')
            with patch('schedule_hetero_control.Jenkins',
                       autospec=True) as jenkins_mock:
                build_jobs(credentials, root, [])
            jenkins_mock.assert_called_once_with(
                'http://localhost:8080', 'jrandom', 'password1')

    def test_get_candidate_version(self):
        with temp_dir() as dir_path:
            self.make_build_var_file(dir_path)
            version = get_candidate_version(dir_path)
        self.assertEqual(version, '1.24.3')

    def test_calculate_jobs(self):
        with temp_dir() as root:
                release_path = os.path.join(root, 'old-juju', '1.20.11')
                os.makedirs(release_path)
                candidate_path = os.path.join(root, 'candidate', '1.22')
                os.makedirs(candidate_path)
                jobs = []
                self.make_build_var_file(candidate_path)
                for job in calculate_jobs(root):
                    jobs.append(job)
        expected = [{'new_to_old': 'true',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3'},
                    {'new_to_old': 'false',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3'}]
        self.assertItemsEqual(jobs, expected)

    def make_build_var_file(self, dir_path):
        build_vars = {"version": "1.24.3", "revision_build": "2870"}
        file_path = os.path.join(dir_path, 'buildvars.json')
        with open(file_path, 'w') as json_file:
            json.dump(build_vars, json_file)
