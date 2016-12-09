#!/usr/bin/env python

from contextlib import contextmanager
import os
from mock import (
    call,
    patch
    )
from unittest import TestCase

from generate_agents import (
    get_agent_version,
    make_centos_agent,
    make_windows_agent,
    move_debs,
    NoDebsFound,
    retrieve_deb_packages,
    retrieve_packages,
    )
from utils import temp_dir


class TestMoveDebs(TestCase):

    def test_juju_2(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju-2.0')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            move_debs(dest_debs)
            self.assertTrue(os.path.exists(os.path.join(dest_debs, 'foo.deb')))

    def test_juju_core(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju-core')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            move_debs(dest_debs)
            self.assertTrue(os.path.exists(os.path.join(dest_debs, 'foo.deb')))

    def test_none(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'juju-core')
            os.mkdir(parent)
            with self.assertRaisesRegexp(NoDebsFound, 'No deb files found.'):
                move_debs(dest_debs)

    def test_wrong_dir(self):
        with temp_dir() as dest_debs:
            parent = os.path.join(dest_debs, 'wrong-dir')
            os.mkdir(parent)
            open(os.path.join(parent, 'foo.deb'), 'w').close()
            with self.assertRaisesRegexp(NoDebsFound, 'No deb files found.'):
                move_debs(dest_debs)


class TestRetreiveDebPackages(TestCase):

    archives = [
        'https://me:secret@private-ppa.launchpad.net/team/stable',
        'https://me:secret@private-ppa.launchpad.net/team/devel',
        ]

    def test_retrieve_deb_packages_stable(self):
        with patch('subprocess.call', autospec=True) as c_mock:
            retrieve_deb_packages('2.1.0', '1', self.archives, '2.1.0/')
        calls = [
            call(['lftp', '-c', 'mirror', '-i',
                  '(juju-2.0|juju-core).*2.1.0.*\\.1~juj.*\\.deb$',
                  'https://me:secret@private-ppa.launchpad.net/team/stable'],
                 cwd='2.1.0/'),
            call(['lftp', '-c', 'mirror', '-i',
                  '(juju-2.0|juju-core).*2.1.0.*\\.1~juj.*\\.deb$',
                  'https://me:secret@private-ppa.launchpad.net/team/devel'],
                 cwd='2.1.0/')
            ]
        self.assertEqual(calls, c_mock.mock_calls)

    def test_retrieve_deb_packages_devel(self):
        with patch('subprocess.call', autospec=True) as c_mock:
            retrieve_deb_packages(
                '2.1-beta1', '1', self.archives, '2.1-beta1/')
        calls = [
            call(['lftp', '-c', 'mirror', '-i',
                  '(juju-2.0|juju-core).*2.1~beta1.*\\.1~juj.*\\.deb$',
                  'https://me:secret@private-ppa.launchpad.net/team/stable'],
                 cwd='2.1-beta1/'),
            call(['lftp', '-c', 'mirror', '-i',
                  '(juju-2.0|juju-core).*2.1~beta1.*\\.1~juj.*\\.deb$',
                  'https://me:secret@private-ppa.launchpad.net/team/devel'],
                 cwd='2.1-beta1/')
            ]
        self.assertEqual(calls, c_mock.mock_calls)


control_string_template = """\
Package: juju-2.0
Source: juju-core
Version: {}
Architecture: amd64
Maintainer: Curtis Hovey <curtis.hovey@canonical.com>
Installed-Size: 405884
Depends: distro-info, libc6 (>= 2.9)
Recommends: bash-completion
Conflicts: juju2
Breaks: juju-core (<= 1.25.5)
Section: devel
Priority: extra
Homepage: http://launchpad.net/juju-core
Description: Juju is devops distilled - client
 Through the use of charms, juju provides you with shareable, re-usable,
 and repeatable expressions of devops best practices. You can use them
 unmodified, or easily change and connect them to fit your needs. Deploying
 a charm is similar to installing a package on Ubuntu: ask for it and
 it's there, remove it and it's completely gone.
 .
 This package provides the client application of creating and interacting
 with Juju environments.
"""


class TestRetrievePackages(TestCase):

    @contextmanager
    def patch_for_test(self):
        with patch('generate_agents.print') as print_mock:
            with patch('generate_agents.datetime') as time_mock:
                with patch('generate_agents.retrieve_deb_packages',
                           autospec=True) as deb_mock:
                    with patch('generate_agents.move_debs') as move_mock:
                        yield (print_mock, time_mock, deb_mock, move_mock)

    @contextmanager
    def mock_s3_config(self):
        with temp_dir() as root_dir:
            config = os.path.join(root_dir, 's3.config')
            with open(config, 'w') as file:
                file.write('Fake Contents')
            yield config

    def test_retrieve_packages(self):
        with self.patch_for_test():
            with self.mock_s3_config() as s3_config:
                with patch('agent_archive.get_agents') as get_agent_mock:
                    retrieve_packages('release', 'upatch', 'archives',
                                      'dest_debs', s3_config)
        self.assertEqual(1, get_agent_mock.call_count)
        args = get_agent_mock.call_args[0][0]
        self.assertEqual(args.version, 'release')
        self.assertEqual(args.destination, 'dest_debs')
        self.assertEqual(args.config, s3_config)


class TestGetAgentVersion(TestCase):

    def test_get_agent_version_stable(self):
        control_str = control_string_template.format(
            '1:2.1.0-0ubuntu1~16.04.1~juju1')
        agent_version = get_agent_version(control_str)
        self.assertEqual('2.1.0', agent_version.version)
        self.assertEqual('2.1', agent_version.major_minor)
        self.assertEqual('xenial', agent_version.series)
        self.assertEqual('amd64', agent_version.architecture)

    def test_get_agent_version_devel(self):
        control_str = control_string_template.format(
            '1:2.1~beta1-0ubuntu1~16.04.1~juju1')
        agent_version = get_agent_version(control_str)
        self.assertEqual('2.1-beta1', agent_version.version)
        self.assertEqual('2.1', agent_version.major_minor)
        self.assertEqual('xenial', agent_version.series)
        self.assertEqual('amd64', agent_version.architecture)


class TestMakeCentosAgent(TestCase):

    def test_make_centos_agent(self):
        with temp_dir() as workspace:
            dest_debs = os.path.join(workspace, 'debs')
            agent_dir = os.path.join(dest_debs, 'agent', '1.25.5')
            os.makedirs(agent_dir)
            stanzas = os.path.join(workspace, 'stanzas')
            os.mkdir(stanzas)
            agent = os.path.join(dest_debs, 'juju-1.25.5-centos7-amd64.tgz')
            with open(agent, 'w') as dummy_file:
                dummy_file.write('centos7 agent')
            make_centos_agent(dest_debs, 'proposed', '1.25.5')
            agent_path = os.path.join(
                workspace, 'debs', 'agent', '1.25.5',
                'juju-1.25.5-centos7-amd64.tgz')
            self.assertTrue(os.path.exists(agent_path))
            stanza_path = os.path.join(
                workspace, 'debs', 'proposed-1.25.5-centos.json')
            self.assertTrue(os.path.exists(stanza_path))


class TestMakeWindowsAgent(TestCase):

    def test_make_windows_agent(self):
        with temp_dir() as workspace:
            dest_debs = os.path.join(workspace, 'debs')
            agent_dir = os.path.join(dest_debs, 'agent', '1.25.5')
            os.makedirs(agent_dir)
            stanzas = os.path.join(workspace, 'stanzas')
            os.mkdir(stanzas)
            agent = os.path.join(dest_debs, 'juju-1.25.5-win2012-amd64.tgz')
            with open(agent, 'w') as dummy_file:
                dummy_file.write('windows agent')
            make_windows_agent(dest_debs, 'proposed', '1.25.5')
            agent_path = os.path.join(
                workspace, 'debs', 'agent', '1.25.5',
                'juju-1.25.5-windows-amd64.tgz')
            self.assertTrue(os.path.exists(agent_path))
            stanza_path = os.path.join(
                workspace, 'debs', 'proposed-1.25.5-windows.json')
            self.assertTrue(os.path.exists(stanza_path))
