from reviewtest.listutil import average, last_n


def test_average():
    assert average([2, 4]) == 3


def test_last_n():
    assert last_n([1, 2, 3, 4], 2)
