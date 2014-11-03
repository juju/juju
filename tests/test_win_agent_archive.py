import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from win_agent_archive import (
    main,
    validate_souce_agent,
)


class WinAgentArchive(TestCase):

    def test_main_add(self):
        with patch('win_agent_archive.add_agents') as mock:
            main(['add', 'path/juju-1.21.0-win2012-amd64.tgz'])
            all_args, kwargs = mock.call_args
            args = all_args[0]
            self.assertEqual(
                'path/juju-1.21.0-win2012-amd64.tgz', args.source_agent)
            self.assertFalse(args.verbose)
            self.assertFalse(args.dry_run)

    def test_main_get(self):
        with patch('win_agent_archive.get_agents') as mock:
            main(['get', '1.21.0'])
            all_args, kwargs = mock.call_args
            args = all_args[0]
            self.assertEqual('1.21.0', args.version)
            self.assertFalse(args.verbose)
            self.assertFalse(args.dry_run)

    def test_validate_souce_agent(self):
        self.assertTrue(validate_souce_agent('juju-1.21.0-win2012-amd64.tgz'))
        self.assertTrue(validate_souce_agent('juju-1.21.0-win9.1-amd64.tgz'))
        self.assertTrue(
            validate_souce_agent('juju-1.21-alpha3-win2012-amd64.tgz'))
        self.assertFalse(validate_souce_agent('juju-1.21.0-trusty-amd64.tgz'))
        self.assertFalse(validate_souce_agent('1.21.0-win2012-amd64.tgz'))
        self.assertFalse(validate_souce_agent('juju-1.21.0-win2012-386.tgz'))
        self.assertFalse(
            validate_souce_agent('juju-1.21.0-win2012-amd64.tar.gz'))
