#!/usr/bin/env python3
"""Simple looped check over provided file for regex content."""
from __future__ import print_function

import argparse
import os
import subprocess
import sys
import time


class check_result:
    success = 0
    failure = 1
    exception = 2


def check_file(check_string, file_path):
    print('Checking for:\n{}'.format(check_string))
    for _ in range(0, 10):
        try:
            subprocess.check_call(
                ['sudo', 'egrep', check_string, file_path])
            print('Log content found. No need to continue.')
            return check_result.success
        except subprocess.CalledProcessError as e:
            if e.returncode == 1:
                time.sleep(1)
            else:
                return check_result.exception
    print('Unexpected error with file check.')
    return check_result.failure


def raise_if_file_not_found(file_path):
    if not os.path.exists(file_path):
        raise ValueError('File not found: {}'.format(file_path))


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description='File content check.')
    parser.add_argument(
        'regex', help='Regex string to check file with.')
    parser.add_argument(
        'file_path', help='Path to file to check.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    try:
        raise_if_file_not_found(args.file_path)
    except ValueError as e:
        print(e)
        sys.exit(check_result.exception)

    sys.exit(check_file(args.regex, args.file_path))


if __name__ == '__main__':
    sys.exit(main())
