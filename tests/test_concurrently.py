import logging
from mock import patch

import concurrently
from tests import (
    TestCase,
)


class ConcurrentlyTest(TestCase):

    log_level = logging.ERROR

    def test_main(self):
        with patch('concurrently.run_all') as r_mock:
            with patch('concurrently.summarise_tasks',
                       return_value=0) as s_mock:
                returncode = concurrently.main(
                    ['-v', 'one=foo a b', 'two=bar c'])
        self.assertEqual(0, returncode)
        task_one = concurrently.Task('one=foo a b')
        task_two = concurrently.Task('two=bar c')
        r_mock.assert_called_once_with([task_one, task_two])
        s_mock.assert_called_once_with([task_one, task_two])

    @patch('sys.stderr')
    def test_main_error(self, se_mock):
        with patch('concurrently.run_all', side_effect=ValueError('bad')):
            returncode = concurrently.main(['-v', 'one=foo a b', 'two=bar c'])
        self.assertEqual(253, returncode)
        self.assertIn('ERROR bad', self.log_stream.getvalue())

    def test_parse_args(self):
        args = concurrently.parse_args(['-v', 'one=foo a b', 'two=bar c'])
        self.assertEqual(logging.DEBUG, args.verbose)
        self.assertEqual(
            ['one=foo a b', 'two=bar c'], args.tasks)

    def test_summarise_tasks(self):
        task_one = concurrently.Task('one=foo a')
        task_one.returncode = 0
        task_two = concurrently.Task('two=bar b')
        task_two.returncode = 0
        tasks = [task_one, task_two]
        self.assertEqual(0, concurrently.summarise_tasks(tasks))
        task_one.returncode = 1
        self.assertEqual(1, concurrently.summarise_tasks(tasks))
        task_two.returncode = 3
        self.assertEqual(4, concurrently.summarise_tasks(tasks))

    def test_run_all(self):
        task_one = concurrently.Task('one=foo a')
        task_two = concurrently.Task('two=bar b')
        mutable_tasks = [task_one, task_two]
        with patch.object(task_one, 'finish', autospec=True) as f1_mock:
            with patch.object(task_one, 'start', autospec=True) as s1_mock:
                with patch.object(task_two, 'finish',
                                  autospec=True,) as f2_mock:
                    with patch.object(task_two, 'start',
                                      autospec=True) as s2_mock:
                        concurrently.run_all(mutable_tasks)
        s1_mock.assert_called_once_with()
        f1_mock.assert_called_once_with()
        s2_mock.assert_called_once_with()
        f2_mock.assert_called_once_with()
        self.assertEqual([], mutable_tasks)


class TaskTest(TestCase):

    def test_init(self):
        task = concurrently.Task('one=foo a b c')
        self.assertEqual('one', task.name)
        self.assertEqual('foo a b c', task.commandline)
        self.assertEqual(['foo', 'a', 'b', 'c'], task.command)
        self.assertEqual('one-out.log', task.out_log_name)
        self.assertEqual('one-err.log', task.err_log_name)
        self.assertIsNone(task.returncode)
        self.assertIsNone(task.proc)
