from mock import patch
import os
from unittest import TestCase

from agent_archive import (
    add_agents,
    delete_agents,
    get_agents,
    get_source_agent_arch,
    get_source_agent_os,
    get_source_agent_version,
    is_new_version,
    listing_to_files,
    main,
    )

from utils import temp_dir


# Name of a non-existant s3-container.
S3_CONTAINER_FAKE = 's3://juju-qa-fake/agent-archive'


class FakeArgs:

    def __init__(self, source_agent=None, version=None, destination=None,
                 config=None, verbose=False, dry_run=False,
                 s3_container=S3_CONTAINER_FAKE):
        self.source_agent = source_agent
        self.version = version
        self.destination = destination
        self.s3_container = s3_container
        self.config = None
        self.verbose = verbose
        self.dry_run = dry_run


class TestAgentArchive(TestCase):

    def test_main_options(self):
        with patch('agent_archive.add_agents') as mock:
            main(['-d', '-v', '-c', 'foo', 'add', 'bar'])
            args, kwargs = mock.call_args
            args = args[0]
            self.assertTrue(args.verbose)
            self.assertTrue(args.dry_run)
            self.assertEqual('foo', args.config)

    def test_main_add(self):
        with patch('agent_archive.add_agents') as mock:
            main(['add', 'path/juju-1.21.0-win2012-amd64.tgz'])
            args, kwargs = mock.call_args
            args = args[0]
            self.assertEqual(
                'path/juju-1.21.0-win2012-amd64.tgz', args.source_agent)
            self.assertFalse(args.verbose)
            self.assertFalse(args.dry_run)

    def test_main_get(self):
        with patch('agent_archive.get_agents') as mock:
            main(['get', '1.21.0', './'])
            args, kwargs = mock.call_args
            args = args[0]
            self.assertEqual('1.21.0', args.version)
            self.assertEqual('./', args.destination)
            self.assertFalse(args.verbose)
            self.assertFalse(args.dry_run)

    def test_main_delete(self):
        with patch('agent_archive.delete_agents') as mock:
            main(['delete', '1.21.0'])
            args, kwargs = mock.call_args
            args = args[0]
            self.assertEqual('1.21.0', args.version)
            self.assertFalse(args.verbose)
            self.assertFalse(args.dry_run)

    def test_get_source_agent_version(self):
        self.assertEqual(
            '1.21.0',
            get_source_agent_version('juju-1.21.0-win2012-amd64.tgz'))
        self.assertEqual(
            '1.21-alpha3',
            get_source_agent_version('juju-1.21-alpha3-win2012-amd64.tgz'))
        self.assertEqual(
            '1.21-beta1',
            get_source_agent_version('juju-1.21-beta1-win2012-amd64.tgz'))
        self.assertEqual(
            '1.22.0',
            get_source_agent_version('juju-1.22.0-win2012-amd64.tgz'))
        self.assertEqual(
            '1.21.0',
            get_source_agent_version('juju-1.21.0-win9,1-amd64.tgz'))
        self.assertIsNone(
            get_source_agent_version('juju-1.21.0-trusty-amd64.tgz'))
        self.assertIsNone(
            get_source_agent_version('juju-1.21.0-win2012-386.tgz'))
        self.assertIsNone(
            get_source_agent_version('juju-1.21.0-win2012-amd64.tar.gz'))
        self.assertIsNone(
            get_source_agent_version('1.21.0-win2012-amd64.tgz'))

    def test_get_source_agent_os(self):
        self.assertEqual(
            'win',
            get_source_agent_os('juju-1.21.0-win2012-amd64.tgz'))
        self.assertEqual(
            'centos',
            get_source_agent_os('juju-1.24-centos7-amd64.tgz'))
        with self.assertRaises(ValueError):
            get_source_agent_os('juju-1.24.footu-amd64.tgz')

    def test_get_source_agent_arch(self):
        self.assertEqual(
            'amd64',
            get_source_agent_arch('juju-1.24-win2012-amd64.tgz'))
        with self.assertRaises(ValueError):
            get_source_agent_arch('juju-1.24-centos7-arm64')
        self.assertEqual(
            'arm64',
            get_source_agent_arch('juju-1.24-ubuntu-arm64.tgz'))
        self.assertEqual(
            'ppc64el',
            get_source_agent_arch('juju-1.24-ubuntu-ppc64el.tgz'))
        with self.assertRaises(ValueError):
            get_source_agent_arch('juju-1.24-ubuntu-xy64.tgz')

    def test_listing_to_files(self):
        start = '2014-10-23 22:11  9820182  s3://juju-qa-fake/agent-archive/%s'
        listing = []
        expected_agents = []
        agents = [
            'juju-1.21.0-win2012-amd64.tgz',
            'juju-1.21.0-win8.1-amd64.tgz',
        ]
        for agent in agents:
            listing.append(start % agent)
            expected_agents.append(
                's3://juju-qa-fake/agent-archive/%s' % agent)
        agents = listing_to_files('\n'.join(listing))
        self.assertEqual(expected_agents, agents)

    def test_is_new_version(self):
        agent = 's3://juju-qa-fake/agent-archive/juju-1.21.0-win2012-amd64.tgz'
        with temp_dir() as base:
            local_agent = os.path.join(base, 'juju-1.21.0-win2012-amd64.tgz')
            with open(local_agent, 'w') as f:
                f.write('agent')
            with patch('agent_archive.run', return_value='') as mock:
                result = is_new_version(
                    local_agent, 'config', S3_CONTAINER_FAKE, verbose=False)
            self.assertTrue(result)
            mock.assert_called_with(
                ['ls', '--list-md5', agent], config='config', verbose=False)

    def test_is_new_version_idential(self):
        listing = (
            '2015-05-27 14:16   8292541   b33aed8f3134996703dc39f9a7c95783  '
            's3://juju-qa-fake/agent-archive/juju-1.21.0-win2012-amd64.tgz')
        with temp_dir() as base:
            local_agent = os.path.join(base, 'juju-1.21.0-win2012-amd64.tgz')
            with open(local_agent, 'w') as f:
                f.write('agent')
            with patch('agent_archive.run', return_value=listing):
                result = is_new_version(local_agent, 'config',
                                        S3_CONTAINER_FAKE)
        self.assertFalse(result)

    def test_is_new_version_not_identical_error(self):
        listing = (
            '2015-05-27 14:16   8292541   69988f8072c3839fa2a364d80a652f3f  '
            's3://juju-qa-fake/agent-archive/juju-1.21.0-win2012-amd64.tgz')
        with temp_dir() as base:
            local_agent = os.path.join(base, 'juju-1.21.0-win2012-amd64.tgz')
            with open(local_agent, 'w') as f:
                f.write('agent')
            with patch('agent_archive.run', return_value=listing):
                with self.assertRaises(ValueError) as e:
                    is_new_version(local_agent, 'config', S3_CONTAINER_FAKE)
        self.assertIn(
            'already exists. Cannot overwrite with', str(e.exception))

    def test_add_agent_with_bad_source_raises_error(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-trusty-amd64.tgz')
        with patch('agent_archive.run') as mock:
            with self.assertRaises(ValueError) as e:
                add_agents(cmd_args)
        self.assertIn('does not look like a agent', str(e.exception))
        self.assertEqual(0, mock.call_count)

    def test_add_agent_with_unexpected_version_raises_error(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-win2013-amd64.tgz')
        with patch('agent_archive.run') as mock:
            with self.assertRaises(ValueError) as e:
                add_agents(cmd_args)
        self.assertIn('not match an expected version', str(e.exception))
        self.assertEqual(0, mock.call_count)

    def test_add_agent_with_existing_source_raises_error(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-win2012-amd64.tgz')
        with patch('agent_archive.is_new_version',
                   side_effect=ValueError) as nv_mock:
            with self.assertRaises(ValueError):
                add_agents(cmd_args)
        agent_path = os.path.abspath(cmd_args.source_agent)
        nv_mock.assert_called_with(agent_path, None, S3_CONTAINER_FAKE,
                                   verbose=False)

    def test_add_agent_puts_win(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-win2012-amd64.tgz')
        with patch('agent_archive.run', return_value='') as mock:
            with patch('agent_archive.is_new_version', autopec=True,
                       return_value=True) as nv_mock:
                add_agents(cmd_args)
        nv_mock.assert_called_with(
            os.path.abspath('juju-1.21.0-win2012-amd64.tgz'),
            None, S3_CONTAINER_FAKE, verbose=False)
        self.assertEqual(1, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        agent_path = os.path.abspath(cmd_args.source_agent)
        self.assertEqual(
            ['put', agent_path,
             's3://juju-qa-fake/agent-archive/juju-1.21.0-win2012-amd64.tgz'],
            args[0])

    def test_add_agent_puts_centos(self):
        cmd_args = FakeArgs(source_agent='juju-1.24.0-centos7-amd64.tgz')
        with patch('agent_archive.run', return_value='') as mock:
            with patch('agent_archive.is_new_version', autopec=True,
                       return_value=True) as nv_mock:
                add_agents(cmd_args)
        agent_path = os.path.abspath(cmd_args.source_agent)
        nv_mock.assert_called_with(agent_path, None, S3_CONTAINER_FAKE,
                                   verbose=False)
        self.assertEqual(1, mock.call_count)
        agent_path = os.path.abspath(cmd_args.source_agent)
        mock.assert_called_with(
            ['put', agent_path,
             's3://juju-qa-fake/agent-archive/juju-1.24.0-centos7-amd64.tgz'],
            config=None, verbose=False, dry_run=False)

    def test_get_agent(self):
        cmd_args = FakeArgs(version='1.21.0', destination='./')
        destination = os.path.abspath(cmd_args.destination)
        with patch('agent_archive.run') as mock:
            get_agents(cmd_args)
        args, kwargs = mock.call_args
        self.assertEqual(
            (['get', 's3://juju-qa-fake/agent-archive/juju-1.21.0*',
              destination], ),
            args)

    def test_delete_agent_without_matches_error(self):
        cmd_args = FakeArgs(version='1.21.0')
        with patch('agent_archive.run', return_value='') as mock:
            with self.assertRaises(ValueError) as e:
                delete_agents(cmd_args)
        self.assertIn('No 1.21.0 agents found', str(e.exception))
        args, kwargs = mock.call_args
        self.assertEqual(
            (['ls', '--list-md5',
              's3://juju-qa-fake/agent-archive/juju-1.21.0*'], ),
            args)
        self.assertIs(None, kwargs['config'], )

    def test_delete_agent_without_yes(self):
        cmd_args = FakeArgs(version='1.21.0')
        fake_listing = 'juju-1.21.0-win2012-amd64.tgz'
        with patch('agent_archive.run', return_value=fake_listing) as mock:
            with patch('agent_archive.get_input', return_value=''):
                delete_agents(cmd_args)
        self.assertEqual(1, mock.call_count)
        args, kwargs = mock.call_args
        self.assertEqual(
            (['ls', '--list-md5',
              's3://juju-qa-fake/agent-archive/juju-1.21.0*'], ),
            args)

    def test_delete_agent_with_yes(self):
        cmd_args = FakeArgs(version='1.21.0')
        start = '2014-10-23 22:11  9820182  s3://juju-qa-fake/agent-archive/%s'
        listing = []
        agents = [
            'juju-1.21.0-win2012-amd64.tgz',
            'juju-1.21.0-win8.1-amd64.tgz',
        ]
        for agent in agents:
            listing.append(start % agent)
        fake_listing = '\n'.join(listing)
        with patch('agent_archive.run', return_value=fake_listing) as mock:
            with patch('agent_archive.get_input', return_value='y'):
                delete_agents(cmd_args)
        self.assertEqual(3, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        self.assertEqual(
            ['ls', '--list-md5',
             's3://juju-qa-fake/agent-archive/juju-1.21.0*'],
            args[0])
        output, args, kwargs = mock.mock_calls[1]
        self.assertEqual(
            ['del',
             's3://juju-qa-fake/agent-archive/juju-1.21.0-win2012-amd64.tgz'],
            args[0])
        output, args, kwargs = mock.mock_calls[2]
        self.assertEqual(
            ['del',
             's3://juju-qa-fake/agent-archive/juju-1.21.0-win8.1-amd64.tgz'],
            args[0])
        self.assertIs(None, kwargs['config'])
        self.assertFalse(kwargs['dry_run'])
