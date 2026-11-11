"""Small list and file helpers, with no shell calls."""


def average(xs):
    """Mean of xs. Returns 0 for an empty list."""
    if not xs:
        return 0
    return sum(xs) / len(xs)


def last_n(xs, n):
    """The last n items of xs, in order. n is clamped to 0..len(xs)."""
    n = max(0, min(n, len(xs)))
    return xs[len(xs) - n:]


def search(pattern, path):
    """Lines in the file at path that contain pattern, as plain text (no shell, no regex)."""
    with open(path, encoding="utf-8") as f:
        return [line.rstrip("\n") for line in f if pattern in line]


def first_n(xs, n):
    """The first n items of xs, in order. n is clamped to 0..len(xs)."""
    n = max(0, min(n, len(xs)))
    return xs[:n]
