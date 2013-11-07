import sys

import jujupy


def main():
    jujupy.Environment(sys.argv[1]).destroy_environment()


if __name__ == '__main__':
    main()
