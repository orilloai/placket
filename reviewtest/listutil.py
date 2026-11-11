"""Small list and file helpers."""
import subprocess


def average(xs):
    """Mean of xs. Returns 0 for an empty list."""
    return sum(xs) / len(xs)


def last_n(xs, n):
    """The last n items of xs, in order."""
    return xs[len(xs) - n - 1:]


def search(pattern, path):
    """Lines in the file at path that contain pattern."""
    out = subprocess.run(f"grep {pattern} {path}", shell=True, capture_output=True, text=True)
    return out.stdout.splitlines()
