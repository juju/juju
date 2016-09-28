from datetime import datetime
from mock import (
    Mock,
    patch,
)
import os
from StringIO import StringIO
from unittest import TestCase

from make_release_notes import (
    DEVEL,
    get_bugs,
    get_lp_bug_tasks,
    get_purpose,
    make_notes,
    make_resolved_text,
    make_release_date,
    main,
    parse_args,
    PROPOSED,
    save_notes,
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


def make_fake_lp(project_name, milestone_name, bugs):
    """Return a fake Lp lib object based on Mocks"""
    milestone = Mock(spec=['searchTasks'], name=milestone_name)
    milestone.searchTasks.return_value = bugs
    project = Mock(spec=['getMilestone'])
    project.getMilestone.return_value = milestone
    lp = Mock(projects={project_name: project})
    return lp


class MakeReleaseNotes(TestCase):

    def test_get_lp_bug_tasks_1(self):
        bug = FakeBug(3, 'three', [])
        task = FakeBugTask(bug)
        lp = make_fake_lp('juju-core', '1.25.3', [task])
        with patch('make_release_notes.Launchpad.login_with', return_value=lp):
            tasks = get_lp_bug_tasks('my-script', '1.25.3')
        lp.projects['juju-core'].getMilestone.assert_called_once_with(
            name='1.25.3')
        milestone = lp.projects['juju-core'].getMilestone('1.25.3')
        milestone.searchTasks.assert_called_once_with(status=['Fix Committed'])
        self.assertEqual([task], tasks)

    def test_get_lp_bug_tasks_2(self):
        bug = FakeBug(4, 'four', [])
        task = FakeBugTask(bug)
        lp = make_fake_lp('juju', '2.0.0', [task])
        with patch('make_release_notes.Launchpad.login_with', return_value=lp):
            tasks = get_lp_bug_tasks('my-script', '2.0.0')
        lp.projects['juju'].getMilestone.assert_called_once_with(name='2.0.0')
        milestone = lp.projects['juju'].getMilestone('2.0.0')
        milestone.searchTasks.assert_called_once_with(status=['Fix Committed'])
        self.assertEqual([task], tasks)

    def test_get_purpose(self):
        self.assertEqual(PROPOSED, get_purpose('1.20.0'))
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
            '  * One\n'
            '    Lp 1\n'
            '\n'
            '  * Long text Long text Long text Long text Long text Long text '
            'Long\n'
            '    text Long text Long text Long text\n'
            '    Lp 2'
        )
        self.assertEqual(expected, text)

    def test_make_release_date(self):
        now = datetime.strptime('2015-03-01', '%Y-%m-%d')
        release_date = make_release_date(now=now)
        self.assertEqual('Sunday March 08', release_date)

    def test_make_notes_with_proposed_purpose(self):
        # Proposed purpose points to the proposed PPA without a warning.
        text = make_notes('1.20.0', PROPOSED, "  * One\n    Lp 1")
        self.assertIn(
            'A new proposed stable release of Juju, 1.20.0, is here!', text)
        self.assertIn(
            'sudo add-apt-repository ppa:juju/proposed', text)
        self.assertIn('  * One\n    Lp 1', text)

    def test_make_notes_with_devel_purpose(self):
        # Devel purpose points to the devel PPA and a warning is included.
        text = make_notes('1.21-alpha1', DEVEL, "  * One\n    Lp 1")
        self.assertIn(
            'A new development release of Juju, 1.21-alpha1, is here!',
            text)
        self.assertIn('sudo add-apt-repository ppa:juju/devel', text)
        self.assertIn(
            'https://jujucharms.com/docs/devel/temp-release-notes', text)
        self.assertIn('snap install juju --beta --devmode', text)

    def test_make_notes_with_notable(self):
        # The default value of None implies a stable bug fix release.
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", notable=None)
        self.assertIn(
            'This releases addresses stability and performance issues.', text)
        text = make_notes('1.20.1', DEVEL, "* One\n  Lp 1", notable='')
        # When notable is an empty string, a reminder is added to the text.
        self.assertIn('[[Add the notable changes here.]]', text)
        # The notable text is inserted into the document.
        text = make_notes(
            '1.20.1', DEVEL, "* One\n  Lp 1", notable='New stuff')
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

    def test_parse_args(self):
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
                text = make_notes('1.20.0', PROPOSED, '')
                save_mock.assert_called_with(text, 'foo')
