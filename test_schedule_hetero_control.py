from __future__ import print_function

__metaclass__ = type

from datetime import timedelta
import json
import os
from time import time
from unittest import TestCase

from mock import patch

from jujuci import Credentials
from schedule_hetero_control import (
    build_jobs,
    calculate_jobs,
    get_args,
    get_candidate_version,
    get_releases,
    )
from utility import temp_dir
from test_utility import write_config


class TestScheduleHeteroControl(TestCase):

    def test_get_args_credentials(self):
        args, credentials = get_args(['root', '--user', 'jrandom',
                                      '--password', 'password'])
        self.assertEqual(args.user, 'jrandom')
        self.assertEqual(args.password, 'password')
        self.assertEqual(credentials, Credentials('jrandom', 'password'))

    def test_get_releases(self):
        with temp_dir() as root:
            old_juju_dir = os.path.join(root, 'old-juju')
            supported_juju = os.path.join(old_juju_dir, '1.24.5')
            devel_juju = os.path.join(old_juju_dir, '1.25-alpha1')
            test_juju = os.path.join(old_juju_dir, '1.24.5~')
            for path in (old_juju_dir, supported_juju, devel_juju, test_juju):
                os.mkdir(path)
            found = get_releases(root)
            self.assertEqual(['1.24.5'], list(found))


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


class TestGetCandidateVersion(TestCase):
    def test_get_candidate_version(self):
        with temp_dir() as dir_path:
            make_build_var_file(dir_path, version='1.24.3')
            version = get_candidate_version(dir_path)
        self.assertEqual(version, '1.24.3')


class CalculateJobs(TestCase):
    def test_calculate_jobs(self):
        with temp_dir() as root:
            release_path = os.path.join(root, 'old-juju', '1.20.11')
            os.makedirs(release_path)
            candidate_path = os.path.join(root, 'candidate', '1.24')
            os.makedirs(candidate_path)
            make_build_var_file(candidate_path, version='1.24.3')
            jobs = list(calculate_jobs(root))
        expected = [{'new_to_old': 'true',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3',
                     'candidate_path': '1.24',
                     'client_os': 'ubuntu',
                     },
                    {'new_to_old': 'false',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3',
                     'candidate_path': '1.24',
                     'client_os': 'ubuntu'}]

        self.assertItemsEqual(jobs, expected)

    def test_calculate_jobs_schedule_all(self):
        with temp_dir() as root:
            release_path = os.path.join(root, 'old-juju', '1.20.11')
            os.makedirs(release_path)
            candidate_path = os.path.join(root, 'candidate', '1.24')
            os.makedirs(candidate_path)
            make_build_var_file(candidate_path, '1.24.3')
            candidate_path_2 = os.path.join(root, 'candidate', '1.23')
            os.makedirs(candidate_path_2)
            buildvars_path = make_build_var_file(candidate_path_2, '1.23.3')
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            os.utime(buildvars_path, (time(), a_week_ago))
            jobs = list(calculate_jobs(root, schedule_all=False))
            jobs_schedule_all = list(calculate_jobs(root, schedule_all=True))
        expected = [{'new_to_old': 'true',
                     'client_os': 'ubuntu',
                     'old_version': '1.20.11',
                     'candidate': '1.23.3',
                     'candidate_path': '1.23'},
                    {'new_to_old': 'false',
                     'client_os': 'ubuntu',
                     'old_version': '1.20.11',
                     'candidate': '1.23.3',
                     'candidate_path': '1.23'},
                    {'new_to_old': 'true',
                     'client_os': 'ubuntu',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3',
                     'candidate_path': '1.24'},
                    {'new_to_old': 'false',
                     'client_os': 'ubuntu',
                     'old_version': '1.20.11',
                     'candidate': '1.24.3',
                     'candidate_path': '1.24'}]
        self.assertItemsEqual(jobs, expected[2:])
        self.assertItemsEqual(jobs_schedule_all, expected)

    def test_calculate_jobs_osx(self):
        with temp_dir() as root:
            release_path = os.path.join(root, 'old-juju', '1.20.11')
            os.makedirs(release_path)
            release_path = os.path.join(root, 'old-juju', '1.20.11-osx')
            os.makedirs(release_path)

            candidate_path_1 = os.path.join(root, 'candidate', '1.24.4')
            os.makedirs(candidate_path_1)
            make_build_var_file(candidate_path_1, '1.24.4')

            candidate_path_2 = os.path.join(root, 'candidate', '1.24.4-osx')
            os.makedirs(candidate_path_2)
            make_build_var_file(candidate_path_2, '1.24.4')
            jobs = list(calculate_jobs(root, schedule_all=False))
        expected = [{'candidate': '1.24.4',
                     'candidate_path': '1.24.4',
                     'client_os': 'ubuntu',
                     'new_to_old': 'true',
                     'old_version': '1.20.11'},
                    {'candidate': '1.24.4',
                     'candidate_path': '1.24.4',
                     'client_os': 'ubuntu',
                     'new_to_old': 'false',
                     'old_version': '1.20.11'},
                    {'candidate': '1.24.4',
                     'candidate_path': '1.24.4-osx',
                     'client_os': 'osx',
                     'new_to_old': 'true',
                     'old_version': '1.20.11-osx'},
                    {'candidate': '1.24.4',
                     'candidate_path': '1.24.4-osx',
                     'client_os': 'osx',
                     'new_to_old': 'false',
                     'old_version': '1.20.11-osx'}]
        self.assertItemsEqual(jobs, expected)


def make_build_var_file(dir_path, version):
    build_vars = {"version": version, "revision_build": "2870"}
    file_path = os.path.join(dir_path, 'buildvars.json')
    with open(file_path, 'w') as json_file:
        json.dump(build_vars, json_file)
    return file_path
