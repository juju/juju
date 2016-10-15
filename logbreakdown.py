from datetime import datetime, timedelta
import gzip

LOG_BREAKDOWN_SECONDS = 20
dt_format = '%Y-%m-%d %H:%M:%S'


def breakdown_log_by_timeframes(log_file, event_timestamps):
    """event_timestamps is a list of TimingData objects."""
    # for each event break the time span into chunks of x seconds. the result
    # being a list of tuples of start/end timestamps

    all_log_breakdown = dict()
    for event in event_timestamps:
        event_range_breakdown = _chunk_event_range(event)
        breakdown = get_timerange_logs(log_file, event_range_breakdown)
        range_name = _render_ds_string(event.start, event.end)
        all_log_breakdown[range_name] = breakdown

    return all_log_breakdown


def _chunk_event_range(event):
    range_breakdown = []
    range_start = datetime.strptime(event.start, dt_format)
    range_end = datetime.strptime(event.end, dt_format)

    next_step = range_start + timedelta(seconds=LOG_BREAKDOWN_SECONDS)

    if next_step > range_end:
        range_breakdown.append((range_start, range_end))
    else:
        while next_step < range_end:
            range_breakdown.append((range_start, next_step))

            range_start = next_step
            next_step = range_start + timedelta(
                seconds=LOG_BREAKDOWN_SECONDS)
            # Otherwise there will be overlap.
            range_start += timedelta(seconds=1)

            if next_step >= range_end:
                range_breakdown.append((range_start, range_end))

    return range_breakdown


def _render_ds_string(start, end):
        return '{} - {}'.format(start, end)


def get_timerange_logs(log_file, timestamps):
    log_breakdown = dict()
    previous_line = None
    no_content = None
    with gzip.open(log_file, 'rt') as f:
        log_lines = []
        for log_range in timestamps:
            range_end = log_range[1]
            if no_content is not None:
                # Extend the range until we get something in the logs.
                range_start = no_content
                no_content = None
                range_str = '{} - {} (condensed)'.format(
                    range_start.strftime('%T'), range_end.strftime('%T'))
                # Don't reset log_lines as it may contain previous details.
            else:
                log_lines = []
                range_start = log_range[0]
                range_str = '{} - {}'.format(
                    range_start.strftime('%T'), range_end.strftime('%T'))

            if previous_line:
                if log_line_within_start_range(previous_line, range_start):
                    log_lines.append(previous_line)
                previous_line = None

            for line in f:
                if log_line_within_start_range(line, range_start):
                    break
            else:
                # Likely because the log cuts off before the action is
                # considered complete (i.e. teardown).
                print('LOG: failed to find start line.')
                break

            # It it's out of range of the end range then there is nothing for
            # this time period.
            if not log_line_within_end_range(line, range_end):
                previous_line = line
                no_content = range_start
                continue

            log_lines.append(line)

            for line in f:
                if log_line_within_end_range(line, range_end):
                    log_lines.append(line)
                else:
                    previous_line = line
                    break
            log_breakdown[range_str] = log_lines

    return log_breakdown


def log_line_within_start_range(line, range_start):
    datestamp = extract_date_from_line(line)
    try:
        ds = datetime.strptime(datestamp, dt_format)
    except ValueError:
        # Don't want an early entry point to the logging.
        return False

    if ds > range_start or ds == range_start:
        return True
    return False


def log_line_within_end_range(line, range_end):
    datestamp = extract_date_from_line(line)
    try:
        ds = datetime.strptime(datestamp, dt_format)
    except ValueError:
        # Fine to collect this line as the line doesn't start with a date and
        # is thus a continuation or undated message.
        return True

    if ds < range_end or ds == range_end:
        return True
    return False


def extract_date_from_line(line):
    return " ".join(line.split()[0:2])
