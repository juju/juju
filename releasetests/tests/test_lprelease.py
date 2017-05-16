from mock import (
    Mock,
    patch,
    )
from StringIO import StringIO
from unittest import TestCase

import lprelease


def make_fake_bugtask(id, title):
    bug = Mock(id=id, title=title)
    bugtask = Mock(spec=['lp_save'], bug=bug, status='Fix Committed')
    return bugtask


def make_fake_milestone(name, tasks=[]):
    milestone = Mock(spec=['searchTasks'])
    # the name attribute cannot be set in __init__ because Mock defines it.
    milestone.name = name
    milestone.searchTasks.return_value = tasks
    return milestone


class LpreleaseTestCase(TestCase):

    def test_close_bugs_stable_milestone(self):
        bugtask = make_fake_bugtask('1', 'title')
        milestone = make_fake_milestone('2.1.0', [bugtask])
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, False, False)
        milestone.searchTasks.assert_called_once_with(status='Fix Committed')
        bugtask.lp_save.assert_called_once_with()
        self.assertIn('', out_fake.getvalue())

    def test_close_bugs_stable_milestone_verbose(self):
        bugtask = make_fake_bugtask('1', 'title')
        milestone = make_fake_milestone('2.1.0', [bugtask])
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, False, True)
        milestone.searchTasks.assert_called_once_with(status='Fix Committed')
        bugtask.lp_save.assert_called_once_with()
        self.assertIn('Updating bug 1 [title]', out_fake.getvalue())

    def test_close_bugs_stable_milestone_dryrun(self):
        bugtask = make_fake_bugtask('1', 'title')
        milestone = make_fake_milestone('2.1.0', [bugtask])
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, True, True)
        milestone.searchTasks.assert_called_once_with(status='Fix Committed')
        self.assertEqual(0, bugtask.lp_save.call_count)
        self.assertIn('Updating bug 1 [title]', out_fake.getvalue())
        self.assertIn('', out_fake.getvalue())

    def test_close_bugs_devel_milestone(self):
        milestone = make_fake_milestone('2.1-rc1')
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, False, False)
        self.assertEqual(0, milestone.searchTasks.call_count)
        self.assertIn('Refusing to close bugs', out_fake.getvalue())
        milestone = make_fake_milestone('2.1-alpha1')
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, False, False)
        self.assertEqual(0, milestone.searchTasks.call_count)
        milestone = make_fake_milestone('2.1-beta1')
        with patch('sys.stdout', new_callable=StringIO) as out_fake:
            lprelease.close_bugs(milestone, False, False)
        self.assertEqual(0, milestone.searchTasks.call_count)
