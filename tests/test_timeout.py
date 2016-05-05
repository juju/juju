from argparse import Namespace
from contextlib import contextmanager
import datetime
import random
from signal import SIGTERM
from unittest import TestCase

from mock import (
    call,
    patch,
    )

from timeout import (
    main,
    parse_args,
    run_command,
    signals,
    )
from tests import parse_error


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args, command = parse_args(['500', 'foo', 'bar'])
        self.assertEqual(args, Namespace(duration=500.0, signal='TERM'))
        self.assertEqual(command, ['foo', 'bar'])

    def test_parse_args_command_options(self):
        args, command = parse_args(['500', 'foo', '--bar'])
        self.assertEqual(args, Namespace(duration=500.0, signal='TERM'))
        self.assertEqual(command, ['foo', '--bar'])

    def test_parse_args_signal(self):
        args, command = parse_args(['500', '--', 'foo', '--signal'])
        self.assertEqual(args, Namespace(duration=500.0, signal='TERM'))
        self.assertEqual(command, ['foo', '--signal'])

    def test_parse_args_signal_novalue(self):
        with parse_error(self) as stderr:
            args, command = parse_args(['500', 'foo', '--signal'])
        self.assertRegexpMatches(
            stderr.getvalue(), 'argument --signal: expected one argument')


class TestMain(TestCase):

    def test_main(self):
        signal_name, signal_value = random.choice(signals.items())
        with patch('timeout.run_command', autospec=True) as rc_mock:
            main(['500', '--signal', signal_name, 'foo', 'bar'])
        rc_mock.assert_called_once_with(500, signal_value, ['foo', 'bar'])


class TestRunCommand(TestCase):

    @contextmanager
    def patch_po(self):
        with patch('subprocess.Popen', autospec=True) as po_mock:
            with patch('time.sleep') as sleep_mock:
                yield po_mock, po_mock.return_value.poll, sleep_mock

    def test_run_and_poll(self):
        with self.patch_po() as (po_mock, poll_mock, sleep_mock):
            poll_mock = po_mock.return_value.poll
            poll_mock.return_value = 123
            self.assertEqual(run_command(57.5, SIGTERM, ['ls', 'foo']),
                             123)
        po_mock.assert_called_once_with(['ls', 'foo'], creationflags=0)
        poll_mock.assert_called_once_with()
        self.assertEqual(sleep_mock.call_count, 0)

    def test_multiple_polls(self):
        with self.patch_po() as (po_mock, poll_mock, sleep_mock):
            poll_mock.side_effect = [None, None, 123, 124]
            self.assertEqual(run_command(57.5, SIGTERM, ['ls', 'foo']),
                             123)
        self.assertEqual(
            poll_mock.mock_calls, [call(), call(), call()])
        self.assertEqual(
            sleep_mock.mock_calls, [call(0.1), call(0.1)])

    def test_duration_elapsed(self):
        start = datetime.datetime(2015, 1, 1)
        middle = start + datetime.timedelta(seconds=57.4)
        end = start + datetime.timedelta(seconds=57.6)
        with self.patch_po() as (po_mock, poll_mock, sleep_mock):
            poll_mock.side_effect = [None, None, None, None]
            with patch('utility.until_timeout.now') as utn_mock:
                utn_mock.side_effect = [start, middle, end, end]
                self.assertEqual(run_command(57.5, SIGTERM, ['ls', 'foo']),
                                 124)
        self.assertEqual(
            poll_mock.mock_calls, [call(), call()])
        self.assertEqual(
            sleep_mock.mock_calls, [call(0.1), call(0.1)])
        self.assertEqual(utn_mock.mock_calls, [call(), call(), call()])
        po_mock.return_value.send_signal.assert_called_once_with(SIGTERM)
        po_mock.return_value.wait.assert_called_once_with()
