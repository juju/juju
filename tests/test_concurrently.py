import logging
from mock import (
    Mock,
    patch,
    )
import os

import concurrently
from tests import (
    parse_error,
    TestCase,
)
from utility import temp_dir


class ConcurrentlyTest(TestCase):

    log_level = logging.ERROR

    def test_main(self):
        with patch('concurrently.run_all', autospec=True) as r_mock:
            with patch('concurrently.summarise_tasks', autospec=True,
                       return_value=0) as s_mock:
                returncode = concurrently.main(
                    ['-v', '-l', '.', 'one=foo a b', 'two=bar c'])
        self.assertEqual(0, returncode)
        task_one = concurrently.Task.from_arg('one=foo a b')
        task_two = concurrently.Task.from_arg('two=bar c')
        r_mock.assert_called_once_with([task_one, task_two])
        s_mock.assert_called_once_with([task_one, task_two])

    def test_main_error(self):
        with patch('concurrently.run_all', side_effect=ValueError('bad')):
            returncode = concurrently.main(['-v', 'one=foo a b', 'two=bar c'])
        self.assertEqual(126, returncode)
        self.assertIn('ERROR Script failed', self.log_stream.getvalue())
        self.assertIn('ValueError: bad', self.log_stream.getvalue())

    def test_bad_task_missing_name(self):
        with parse_error(self) as err_stream:
            concurrently.main(['-v', 'bad'])
        self.assertIn(
            "invalid task_definition value: 'bad'", err_stream.getvalue())
        self.assertEqual('', self.log_stream.getvalue())

    def test_bad_task_bad_lex(self):
        with parse_error(self) as err_stream:
            concurrently.main(['-v', 'wrong="command'])
        self.assertIn(
            """invalid task_definition value: 'wrong="command'""",
            err_stream.getvalue())
        self.assertEqual('', self.log_stream.getvalue())

    def test_max_failure_returncode(self):
        """With many tasks the return code is clamped to under 127."""
        definitions = ["t{}=job".format(i) for i in range(101)]
        with patch('concurrently.run_all') as r_mock:
            with patch('concurrently.summarise_tasks',
                       return_value=101) as s_mock:
                returncode = concurrently.main(['-v', '-l', '.'] + definitions)
        self.assertEqual(100, returncode)
        tasks = map(concurrently.Task.from_arg, definitions)
        r_mock.assert_called_once_with(tasks)
        s_mock.assert_called_once_with(tasks)

    def test_parse_args(self):
        args = concurrently.parse_args(
            ['-v', '-l', '~/logs', 'one=foo a b', 'two=bar c'])
        self.assertEqual(logging.DEBUG, args.verbose)
        self.assertEqual(os.path.expanduser('~/logs'), args.log_dir)
        self.assertEqual(
            [('one', ['foo', 'a', 'b']), ('two', ['bar', 'c'])], args.tasks)

    def test_summarise_tasks(self):
        task_one = concurrently.Task.from_arg('one=foo a')
        task_one.returncode = 0
        task_two = concurrently.Task.from_arg('two=bar b')
        task_two.returncode = 0
        tasks = [task_one, task_two]
        self.assertEqual(0, concurrently.summarise_tasks(tasks))
        task_one.returncode = 1
        self.assertEqual(1, concurrently.summarise_tasks(tasks))
        task_two.returncode = 3
        self.assertEqual(2, concurrently.summarise_tasks(tasks))

    def test_summarise_is_not_summing(self):
        """Exit codes must not be 0 for failed tasks when truncated to char."""
        task_one = concurrently.Task.from_arg('one=foo a')
        task_one.returncode = 255
        task_two = concurrently.Task.from_arg('two=bar b')
        task_two.returncode = 1
        tasks = [task_one, task_two]
        self.assertNotEqual(0, concurrently.summarise_tasks(tasks) & 255)

    def test_run_all(self):
        task_one = concurrently.Task.from_arg('one=foo a')
        task_two = concurrently.Task.from_arg('two=bar b')
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
        with temp_dir() as base:
            task = concurrently.Task.from_arg('one=foo a b c', log_dir=base)
            self.assertEqual('one', task.name)
            self.assertEqual(['foo', 'a', 'b', 'c'], task.command)
            self.assertEqual(os.path.join(base, 'one.log'), task.log_name)
            self.assertIsNone(task.returncode)
            self.assertIsNone(task.proc)

    def test_init_quoted_args(self):
        with temp_dir() as base:
            task = concurrently.Task.from_arg('one=foo a "b c"', log_dir=base)
            self.assertEqual('one', task.name)
            self.assertEqual(['foo', 'a', 'b c'], task.command)

    def test_start(self):
        with temp_dir() as base:
            task = concurrently.Task.from_arg('one=foo a', log_dir=base)
            with patch('subprocess.Popen',
                       autospec=True, return_value='proc') as p_mock:
                with task.start() as proc:
                    self.assertEqual('proc', proc)
                    self.assertEqual('proc', task.proc)
                    self.assertEqual(1, p_mock.call_count)
                    args, kwargs = p_mock.call_args
                    self.assertEqual((['foo', 'a'], ), args)
                    kwargs['stdout'].write('out\n')
                    kwargs['stderr'].write('err\n')
            log_path = os.path.join(base, 'one.log')
            self.assertIs(True, os.path.exists(log_path))
            with open(log_path, 'r') as one_log:
                messages = one_log.read().splitlines()
        self.assertIn('out', messages[0])
        self.assertIn('err', messages[1])

    def test_finish(self):
        task = concurrently.Task.from_arg('one=foo a')
        task.proc = Mock(spec=['wait'])
        task.finish()
        task.proc.wait.assert_called_once_with()
