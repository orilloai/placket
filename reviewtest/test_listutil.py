from reviewtest.listutil import average, last_n, search


def test_average():
    assert average([2, 4]) == 3
    assert average([]) == 0


def test_last_n():
    assert last_n([1, 2, 3, 4], 2) == [3, 4]
    assert last_n([1, 2, 3, 4], 0) == []
    assert last_n([1, 2, 3, 4], 4) == [1, 2, 3, 4]
    assert last_n([1, 2, 3, 4], 9) == [1, 2, 3, 4]


def test_search(tmp_path):
    p = tmp_path / "f.txt"
    p.write_text("a b\n; rm -rf x\nc\n", encoding="utf-8")
    assert search("a b", p) == ["a b"]
    assert search("; rm", p) == ["; rm -rf x"]
    assert search("zz", p) == []


def test_first_n():
    from reviewtest.listutil import first_n
    assert first_n([1, 2, 3], 2) == [1, 2]
    assert first_n([1, 2, 3], -1) == []
    assert first_n([1, 2, 3], 9) == [1, 2, 3]
