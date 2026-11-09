import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from fetch import load_proxies, parse_proxy_line  # noqa: E402


def test_parse_proxy_line_webshare_format():
    assert parse_proxy_line("64.137.126.3:6611:user:pass") == "http://user:pass@64.137.126.3:6611"


def test_parse_proxy_line_with_leading_index():
    assert parse_proxy_line("1\t64.137.126.3:6611:user:pass") == "http://user:pass@64.137.126.3:6611"


def test_parse_proxy_line_blank_or_bad_returns_none():
    assert parse_proxy_line("") is None
    assert parse_proxy_line("not-a-proxy") is None
    assert parse_proxy_line("host:port") is None


def test_load_proxies_skips_bad_lines(tmp_path):
    f = tmp_path / "p.txt"
    f.write_text("1.2.3.4:80:u1:p1\n\ngarbage\n5.6.7.8:8080:u2:p2\n")
    got = load_proxies(str(f))
    assert got == [
        "http://u1:p1@1.2.3.4:80",
        "http://u2:p2@5.6.7.8:8080",
    ]


def test_load_proxies_missing_file_returns_empty():
    assert load_proxies("/no/such/file.txt") == []
