__metaclass__ = type

from contextlib import contextmanager
from StringIO import StringIO
from unittest import TestCase

from mock import patch

from industrial_test import (
    IndustrialTest,
    parse_args,
    )


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit) as e_cxt:
        with patch('sys.stderr', stderr):
            yield stderr


class TestParseArgs(TestCase):

    def test_parse_args(self):
        with parse_error(self) as stderr:
            args = parse_args([])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        with parse_error(self) as stderr:
            args = parse_args(['env'])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        args = parse_args(['rai', 'new-juju'])
        self.assertEqual(args.env, 'rai')
        self.assertEqual(args.new_juju_path, 'new-juju')


class TestIndustrialTest(TestCase):

    def test_init(self):
        old_client = object()
        new_client = object()
        industrial = IndustrialTest(old_client, new_client)
        self.assertIs(old_client, industrial.old_client)
        self.assertIs(new_client, industrial.new_client)

    def test_from_args(self):
        side_effect = lambda x, y=None: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            industrial = IndustrialTest.from_args('env-name', 'new-juju-path')
        self.assertIsInstance(industrial, IndustrialTest)
        self.assertEqual(industrial.old_client, ('env-name', None))
        self.assertEqual(industrial.new_client, ('env-name', 'new-juju-path'))
