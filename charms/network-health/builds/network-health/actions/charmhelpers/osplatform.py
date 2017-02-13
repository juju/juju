import platform


def get_platform():
    """Return the current OS platform.

    For example: if current os platform is Ubuntu then a string "ubuntu"
    will be returned (which is the name of the module).
    This string is used to decide which platform module should be imported.
    """
    tuple_platform = platform.linux_distribution()
    current_platform = tuple_platform[0]
    if "Ubuntu" in current_platform:
        return "ubuntu"
    elif "CentOS" in current_platform:
        return "centos"
    else:
        raise RuntimeError("This module is not supported on {}."
                           .format(current_platform))
