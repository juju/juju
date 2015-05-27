from mock import patch
import os
from unittest import TestCase

from agent_archive import (
    add_agents,
    delete_agents,
    get_agents,
    get_source_agent_os,
    get_source_agent_version,
    listing_to_files,
    main,
)


class FakeArgs:

    def __init__(self, source_agent=None, version=None, destination=None,
                 config=None, verbose=False, dry_run=False):
        self.source_agent = source_agent
        self.version = version
        self.destination = destination
        self.config = None
        self.verbose = verbose
        self.dry_run = dry_run


class WinAgentArchive(TestCase):

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
        self.assertIs(
            None,
            get_source_agent_version('juju-1.21.0-trusty-amd64.tgz'))
        self.assertIs(
            None,
            get_source_agent_version('juju-1.21.0-win2012-386.tgz'))
        self.assertIs(
            None,
            get_source_agent_version('juju-1.21.0-win2012-amd64.tar.gz'))
        self.assertIs(
            None,
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

    def test_listing_to_files(self):
        start = '2014-10-23 22:11   9820182   s3://juju-qa-data/win-agents/%s'
        listing = []
        expected_agents = []
        agents = [
            'juju-1.21.0-win2012-amd64.tgz',
            'juju-1.21.0-win8.1-amd64.tgz',
        ]
        for agent in agents:
            listing.append(start % agent)
            expected_agents.append('s3://juju-qa-data/win-agents/%s' % agent)
        agents = listing_to_files('\n'.join(listing))
        self.assertEqual(expected_agents, agents)

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

    def test_add_agent_with_exist_source_raises_error(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-win2012-amd64.tgz')
        output = 's3://juju-qa-data/win-agents/juju-1.21.0-win2012-amd64.tgz'
        with patch('agent_archive.run', return_value=output) as mock:
            with self.assertRaises(ValueError) as e:
                add_agents(cmd_args)
        self.assertIn('Agents cannot be overwritten', str(e.exception))
        args, kwargs = mock.call_args
        self.assertEqual(
            (['ls', 's3://juju-qa-data/win-agents/juju-1.21.0*'], ),
            args)
        self.assertIs(None, kwargs['config'])

    def test_add_agent_puts_and_copies_win(self):
        cmd_args = FakeArgs(source_agent='juju-1.21.0-win2012-amd64.tgz')
        with patch('agent_archive.run', return_value='') as mock:
            add_agents(cmd_args)
        self.assertEqual(8, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        self.assertEqual(
            (['ls', 's3://juju-qa-data/win-agents/juju-1.21.0*'], ),
            args)
        output, args, kwargs = mock.mock_calls[1]
        agent_path = os.path.abspath(cmd_args.source_agent)
        self.assertEqual(
            (['put', agent_path,
              's3://juju-qa-data/win-agents/juju-1.21.0-win2012-amd64.tgz'], ),
            args)
        # The remaining calls after the put is a fast cp to the other names.
        output, args, kwargs = mock.mock_calls[2]
        self.assertEqual(
            (['cp',
             's3://juju-qa-data/win-agents/juju-1.21.0-win2012-amd64.tgz',
             's3://juju-qa-data/win-agents/juju-1.21.0-win2012hvr2-amd64.tgz'
              ], ),
            args)
        output, args, kwargs = mock.mock_calls[7]
        self.assertEqual(
            (['cp',
             's3://juju-qa-data/win-agents/juju-1.21.0-win2012-amd64.tgz',
             's3://juju-qa-data/win-agents/juju-1.21.0-win81-amd64.tgz'], ),
            args)

    def test_add_agent_puts_centos(self):
        cmd_args = FakeArgs(source_agent='juju-1.24.0-centos7-amd64.tgz')
        with patch('agent_archive.run', return_value='') as mock:
            add_agents(cmd_args)
        self.assertEqual(2, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        self.assertEqual(
            (['ls', 's3://juju-qa-data/win-agents/juju-1.24.0*'], ),
            args)
        output, args, kwargs = mock.mock_calls[1]
        agent_path = os.path.abspath(cmd_args.source_agent)
        self.assertEqual(
            (['put', agent_path,
              's3://juju-qa-data/win-agents/juju-1.24.0-centos7-amd64.tgz'], ),
            args)

    def test_get_agent(self):
        cmd_args = FakeArgs(version='1.21.0', destination='./')
        destination = os.path.abspath(cmd_args.destination)
        with patch('agent_archive.run') as mock:
            get_agents(cmd_args)
        args, kwargs = mock.call_args
        self.assertEqual(
            (['get', 's3://juju-qa-data/win-agents/juju-1.21.0*',
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
            (['ls', 's3://juju-qa-data/win-agents/juju-1.21.0*'], ),
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
            (['ls', 's3://juju-qa-data/win-agents/juju-1.21.0*'], ),
            args)

    def test_delete_agent_with_yes(self):
        cmd_args = FakeArgs(version='1.21.0')
        start = '2014-10-23 22:11   9820182   s3://juju-qa-data/win-agents/%s'
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
            (['ls', 's3://juju-qa-data/win-agents/juju-1.21.0*'], ),
            args)
        output, args, kwargs = mock.mock_calls[1]
        self.assertEqual(
            (['del',
             's3://juju-qa-data/win-agents/juju-1.21.0-win2012-amd64.tgz'], ),
            args)
        output, args, kwargs = mock.mock_calls[2]
        self.assertEqual(
            (['del',
             's3://juju-qa-data/win-agents/juju-1.21.0-win8.1-amd64.tgz'], ),
            args)
        self.assertIs(None, kwargs['config'])
        self.assertFalse(kwargs['dry_run'])
