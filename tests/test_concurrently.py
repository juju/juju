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

    def test_parse_args(self):
        args = concurrently.parse_args(['-v', 'one=foo a b', 'two=bar c'])
        self.assertEqual(logging.DEBUG, args.verbose)
        self.assertEqual(
            ['one=foo a b', 'two=bar c'], args.tasks)
