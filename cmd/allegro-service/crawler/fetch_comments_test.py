import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from fetch import comment_targets, select_top_comments  # noqa: E402


def test_comment_targets_caps_and_skips_urlless():
    offers = [
        {"url": "u1"}, {"url": None}, {"url": "u2"}, {}, {"url": "u3"},
    ]
    got = comment_targets(offers, 2)
    assert [o["url"] for o in got] == ["u1", "u2"]


def test_comment_targets_zero_cap_returns_empty():
    assert comment_targets([{"url": "u"}], 0) == []


def test_comment_targets_fewer_than_cap():
    offers = [{"url": "u1"}, {}, {"url": "u2"}]
    got = comment_targets(offers, 10)
    assert [o["url"] for o in got] == ["u1", "u2"]


def test_select_top_comments_sorts_by_helpful_and_caps():
    raw = [
        {"content": "a", "helpful_count": 1},
        {"content": "b", "helpful_count": 9},
        {"content": "c", "helpful_count": None},
        {"content": "d", "helpful_count": 5},
        {"content": "e", "helpful_count": 3},
        {"content": "f", "helpful_count": 7},
    ]
    got = select_top_comments(raw, 5)
    assert [c["content"] for c in got] == ["b", "f", "d", "e", "a"]
    assert [c["position"] for c in got] == [1, 2, 3, 4, 5]


def test_select_top_comments_handles_missing_helpful_count():
    raw = [{"content": "x"}, {"content": "y", "helpful_count": 2}]
    got = select_top_comments(raw, 5)
    assert [c["content"] for c in got] == ["y", "x"]


def test_select_top_comments_empty():
    assert select_top_comments([], 5) == []
