from mock import patch
import os
from StringIO import StringIO
from unittest import TestCase


from make_release_notes import (
    DEVEL,
    get_bugs,
    get_purpose,
    make_notes,
    make_resolved_text,
    main,
    parse_args,
    save_notes,
    STABLE,
)
from utils import temp_dir


class FakeBug(object):

    def __init__(self, id, title, tags):
        self.id = id
        self.title = title
        self.tags = tags


class FakeBugTask(object):

        def __init__(self, bug):
            self.bug = bug


class MakeReleaseNotes(TestCase):

    def test_get_purpose(self):
        self.assertEqual(STABLE, get_purpose('1.20.0'))
        self.assertEqual(DEVEL, get_purpose('1.21-alpha1'))

    def test_get_bugs(self):
        tasks = [
            FakeBugTask(FakeBug('1', 'one one', ['tech-debt'])),
            FakeBugTask(FakeBug(2, 'two two', ['ui']))]
        with patch('make_release_notes.get_lp_bug_tasks',
                   return_value=tasks) as mock:
            bugs = get_bugs('script', '1.20.1')
            mock.assert_called_with('script', '1.20.1')
            self.assertEqual([(2, 'Two two')], bugs)

    def test_make_resolved_text(self):
        text = make_resolved_text([('1', 'One'), ('2', 'Long text ' * 10)])
        expected = (
            '* One\n'
            '  Lp 1\n'
            '\n'
            '* Long text Long text Long text Long text Long text Long text '
            'Long\n'
            '  text Long text Long text Long text\n'
            '  Lp 2'
        )
        self.assertEqual(expected, text)

    def test_make_notes_with_stable_purpose(self):
        # Stable purpose points to the stable PPA without a warning.
        text = make_notes('1.20.0', STABLE, "* One\n  Lp 1")
        self.assertIn(
            'A new stable release of Juju, juju-core 1.20.0, is now available.',
            text)
        self.assertIn(
            'https://launchpad.net/~juju/+archive/stable',
            text)
        self.assertIn(
            '* One\n  Lp 1',
            text)
        self.assertNotIn(
            'Upgrading from stable releases to development releases is not',
            text)

    def test_make_notes_with_devel_purpose(self):
        # Devel purpose points to the devel PPA and a warning is included.
        text = make_notes('1.21-alpha1', DEVEL, "* One\n  Lp 1")
        self.assertIn(
            'A new development release of Juju, juju-core 1.21-alpha1,'
            ' is now available.',
            text)
        self.assertIn(
            'https://launchpad.net/~juju/+archive/devel',
            text)
        self.assertIn(
            '* One\n  Lp 1',
            text)
        self.assertIn(
            'Upgrading from stable releases to development releases is not',
            text)

    def test_make_notes_with_previous(self):
        # The "replaces" text is included when previous is set.
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", previous='1.20.0')
        self.assertIn('This release replaces 1.20.0.', text)
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", previous=None)
        self.assertNotIn('This release replaces', text)

    def test_make_notes_with_notable(self):
        # The default value of None implies a stable bug fix release.
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", notable=None)
        self.assertIn(
            'This releases addresses stability and performance issues.', text)
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", notable='')
        # When notable is an empty string, a reminder is inserted into the test.
        self.assertIn('[[Add the notable changes here.]]', text)
        # The notable text is inserted into the document.
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", notable='New stuff')
        self.assertIn('New stuff', text)

    def test_save_notes(self):
        # The notes are written to file when file_name is not None.
        with temp_dir() as base:
            file_name = '{0}/foo.txt'.format(base)
            save_notes('bar', file_name)
            self.assertTrue(os.path.exists(file_name))
            with open(file_name) as rn:
                content = rn.read()
            self.assertEqual('bar', content)
        # When the file-name  is None, the notes are written to stdout.
        with patch('sys.stdout', new_callable=StringIO) as so_mock:
            save_notes('bar', None)
            self.assertEqual('bar\n', so_mock.getvalue())

    def test_parge_args(self):
        # Only the milestone is required.
        args = parse_args(['foo'])
        self.assertEqual('foo', args.milestone)
        self.assertEqual(None, args.file_name)
        self.assertEqual(None, args.previous)
        # A file_name can be passed to save the release notes too.
        args = parse_args(['--file-name', 'bar', 'foo'])
        # The previous release can be passed for the release notes.
        args = parse_args(['--previous', 'baz', 'foo'])
        self.assertEqual('baz', args.previous)

    def test_main(self):
        with patch('make_release_notes.get_lp_bug_tasks') as lp_mock:
            with patch('make_release_notes.save_notes') as save_mock:
                return_code = main(['script', '--file-name', 'foo', '1.20.0'])
                self.assertEqual(0, return_code)
                lp_mock.assert_called_with('script', '1.20.0')
                text = make_notes('1.20.0', STABLE, '')
                save_mock.assert_called_with(text, 'foo')
