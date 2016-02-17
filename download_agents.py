#!/usr/bin/env python3
from __future__ import print_function

from argparse import ArgumentParser
import errno
import json
import os
from urllib.request import urlopen
import subprocess
import sys


def main():
    parser = ArgumentParser()
    parser.add_argument('downloads_file', metavar='downloads-file')
    args = parser.parse_args()
    with open(args.downloads_file) as downloads_file:
        downloads = json.load(downloads_file)
    for download in downloads:
        path = download['path']
        if os.path.isfile(path):
            print('File already exists: {}'.format(path))
        else:
            print('Downloading: {}'.format(path), end='')
            sys.stdout.flush()
            try:
                os.makedirs(os.path.dirname(download['path']))
            except OSError as e:
                if e.errno != errno.EEXIST:
                    raise
            with open(download['path'], 'wb') as target:
                with urlopen(download['url']) as source:
                    while True:
                        chunk = source.read(102400)
                        if len(chunk) == 0:
                            break
                        target.write(chunk)
                        print('.', end='', file=sys.stderr)
                        sys.stderr.flush()
            print('')
        print('Verifying hash')
        hashsum = subprocess.check_output(
            ['sha256sum', path]).split(b' ', 1)[0]
        hashsum = hashsum.decode('ascii')
        expected = download['sha256']
        print(' {}\n {}'.format(hashsum, expected))
        if hashsum != expected:
            raise ValueError('Incorrect hash for {}'.format(path))


if __name__ == '__main__':
    main()
