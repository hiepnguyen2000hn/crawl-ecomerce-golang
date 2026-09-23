#!/usr/bin/env python3
"""Tách dữ liệu do các live test cào về thành CSV + XLSX riêng cho từng service.

Live test (go test -tags live) lưu JSON thô vào LIVE_OUT_DIR. Script này đọc
các file đó và ghi ra:

    <outdir>/trend-service/               trend_weekly.csv, trend_monthly.csv
    <outdir>/fb-ads-service/              fbads.csv
    <outdir>/product-extractor-service/   products.csv, landing_pages.csv
    <outdir>/niche-research-service/      niche_session.csv, niche_ai_insights.csv,
                                          niche_trend_24m_weekly.csv, niche_trend_24m_monthly.csv

mỗi thư mục kèm một file <service>.xlsx gộp các CSV của nó (qua csv_to_xlsx.py).

Usage: python3 scripts/export_live.py [livedir] [outdir]
       livedir mặc định exports/live, outdir mặc định exports/crawl_<ngày>_<keyword>_<geo>
"""
import csv
import json
import re
import subprocess
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

HERE = Path(__file__).resolve().parent
LIVE = Path(sys.argv[1] if len(sys.argv) > 1 else "exports/live")

# Tên file do internal/livetest.Save sinh ra: <tên>_<YYYYMMDDTHHMMSSZ>.<ext>
STAMP = re.compile(r"_(\d{8}T\d{6}Z)\.\w+$")


def runs(prefix):
    """Các file có tiền tố prefix, cũ trước mới sau."""
    files = [p for p in LIVE.glob(prefix + "*") if STAMP.search(p.name)]
    return sorted(files, key=lambda p: STAMP.search(p.name).group(1))


def latest(prefix):
    files = runs(prefix)
    return files[-1] if files else None


def load(path):
    with path.open(encoding="utf-8") as f:
        return json.load(f)


def write_csv(path, header, rows):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(header)
        w.writerows(rows)
    print(f"  {path}  ({len(rows)} rows)")


def iso(ts):
    if ts in (None, ""):
        return ""
    return datetime.fromtimestamp(int(ts), tz=timezone.utc).strftime("%Y-%m-%d")


def domain(url):
    return urlparse(url).netloc.removeprefix("www.") if url else ""


def timeline(raw):
    """(nhãn tuần, ngày đầu tuần, giá trị, partial) từ response google_trends."""
    out = []
    for p in raw.get("interest_over_time", {}).get("timeline_data", []):
        v = p["values"][0]["extracted_value"] if p.get("values") else ""
        out.append((p["date"], iso(p.get("timestamp")), v, bool(p.get("partial_data"))))
    return out


def monthly(points):
    """Trung bình theo tháng của ngày đầu tuần, bỏ tuần partial (chưa đủ dữ liệu)."""
    buckets = defaultdict(list)
    for _, start, v, partial in points:
        if not partial and start:
            buckets[start[:7]].append(v)
    return [(m, round(sum(vs) / len(vs), 1), len(vs)) for m, vs in sorted(buckets.items())]


def export_trend(out):
    path = latest("trend_")
    if not path:
        print("  (không có dữ liệu trend)")
        return
    raw = load(path)
    params = raw.get("search_parameters", {})
    kw, geo = params.get("q", ""), params.get("geo", "") or "WORLD"
    fetched = raw.get("search_metadata", {}).get("created_at", "")
    pts = timeline(raw)
    write_csv(out / "trend_weekly.csv",
              ["keyword", "geo", "date_range", "week_label", "week_start", "value", "partial_data", "fetched_at"],
              [(kw, geo, params.get("date", ""), lbl, start, v, partial, fetched) for lbl, start, v, partial in pts])
    write_csv(out / "trend_monthly.csv",
              ["keyword", "geo", "month", "avg_value", "weeks"],
              [(kw, geo, m, avg, n) for m, avg, n in monthly(pts)])


def export_fbads(out):
    path = latest("fbads_")
    if not path:
        print("  (không có dữ liệu fbads)")
        return {}
    # fbads_<query>_<country>_<stamp>.json
    stem = STAMP.sub("", path.name).removeprefix("fbads_")
    query, _, country = stem.rpartition("_")
    rows, by_url = [], defaultdict(list)
    for a in load(path):
        s = a.get("snapshot") or {}
        body = (s.get("body") or {}).get("text", "") if isinstance(s.get("body"), dict) else (s.get("body") or "")
        active = a.get("total_active_time")
        impr = a.get("impressions_with_index") or {}
        link = s.get("link_url") or ""
        rows.append((
            query, country, a.get("ad_archive_id", ""),
            f"https://www.facebook.com/ads/library/?id={a.get('ad_archive_id', '')}",
            a.get("page_id", ""), a.get("page_name", ""), s.get("page_like_count", ""), s.get("page_profile_uri", ""),
            a.get("is_active", ""), iso(a.get("start_date")), iso(a.get("end_date")),
            "" if active is None else round(active / 86400, 2),
            impr.get("impressions_text") or "", a.get("reach_estimate") or "", a.get("spend") or "",
            ", ".join(a.get("publisher_platform") or []), s.get("display_format", ""),
            s.get("title") or "", body, s.get("caption") or "", s.get("link_description") or "",
            s.get("cta_text") or "", s.get("cta_type") or "", link, domain(link),
            a.get("collation_count") or "", len(s.get("images") or []), len(s.get("videos") or []),
        ))
        if link:
            by_url[link].append((a.get("page_name", ""), a.get("ad_archive_id", "")))
    write_csv(out / "fbads.csv", [
        "query", "country", "ad_archive_id", "ad_library_url", "page_id", "page_name", "page_like_count",
        "page_profile_uri", "is_active", "start_date", "end_date", "total_active_days", "impressions",
        "reach_estimate", "spend", "publisher_platforms", "display_format", "title", "body", "caption",
        "link_description", "cta_text", "cta_type", "link_url", "link_domain", "collation_count",
        "image_count", "video_count",
    ], rows)
    return by_url


def export_products(out, ads_by_url):
    files = runs("products_extracted")
    if not files:
        print("  (không có dữ liệu products)")
        return
    # Gộp mọi lượt chạy, lượt sau đè lượt trước theo URL. Bản cũ đánh key theo
    # tên site và để URL trong trường "url"; bản mới đánh key theo URL.
    report = {}
    for p in files:
        for key, v in load(p).items():
            report[v.get("url", key)] = v

    products, pages = [], []
    for url, v in report.items():
        owners = ads_by_url.get(url, [])
        page_names = ", ".join(sorted({n for n, _ in owners}))
        ad_ids = ", ".join(i for _, i in owners)
        items = v.get("products") or []
        if v.get("error"):
            status = "error"
        elif items:
            status = "products_found"
        else:
            status = "no_products"
        pages.append((url, domain(url), page_names, len(owners), status, len(items), v.get("error", "")))
        for it in items:
            products.append((url, domain(url), page_names, ad_ids, it.get("product_name", ""),
                             it.get("price", ""), it.get("currency", ""), it.get("sku", "")))
    write_csv(out / "products.csv",
              ["landing_url", "landing_domain", "page_names", "ad_archive_ids", "product_name", "price", "currency", "sku"],
              products)
    write_csv(out / "landing_pages.csv",
              ["landing_url", "landing_domain", "page_names", "ad_count", "status", "product_count", "error"],
              sorted(pages, key=lambda r: (r[4] != "products_found", r[1])))


def export_niche(out):
    path = latest("niche_session_")
    if path:
        s = load(path)
        fields = ["id", "raw_keyword", "country_code", "country_name", "status", "step1_score",
                  "avg_monthly_searches", "demand_type", "demand_label", "volatility", "volatility_label",
                  "fluctuation_ratio", "trend_direction", "kw_source", "volume_source", "ai_source",
                  "seasonality_note", "ai_summary", "last_run_at"]
        row = [s.get(f) if s.get(f) is not None else "" for f in fields]
        write_csv(out / "niche_session.csv",
                  fields + ["peak_months", "low_months"],
                  [row + [", ".join(s.get("peak_months") or []), ", ".join(s.get("low_months") or [])]])
        insights = [("risk", i + 1, t) for i, t in enumerate(s.get("ai_risks") or [])]
        insights += [("action", i + 1, t) for i, t in enumerate(s.get("ai_actions") or [])]
        write_csv(out / "niche_ai_insights.csv", ["type", "no", "text"], insights)
    else:
        print("  (không có niche session — cần LIVE_PG_DSN để chạy TestLive_CreateAndRun)")

    path = latest("niche_trend24m_")
    if path:
        raw = load(path)
        params = raw.get("search_parameters", {})
        kw, geo = params.get("q", ""), params.get("geo", "") or "WORLD"
        pts = timeline(raw)
        write_csv(out / "niche_trend_24m_weekly.csv",
                  ["keyword", "geo", "date_range", "week_label", "week_start", "value", "partial_data"],
                  [(kw, geo, params.get("date", ""), lbl, start, v, partial) for lbl, start, v, partial in pts])
        write_csv(out / "niche_trend_24m_monthly.csv",
                  ["keyword", "geo", "month", "avg_value", "weeks"],
                  [(kw, geo, m, avg, n) for m, avg, n in monthly(pts)])


def to_xlsx(folder):
    if any(folder.glob("*.csv")):
        subprocess.run([sys.executable, str(HERE / "csv_to_xlsx.py"), str(folder), str(folder / f"{folder.name}.xlsx")],
                       check=True, stdout=subprocess.DEVNULL)
        print(f"  {folder / (folder.name + '.xlsx')}")


def main():
    if not LIVE.is_dir():
        sys.exit(f"không thấy thư mục {LIVE}")
    probe = latest("trend_") or latest("niche_trend24m_")
    params = load(probe).get("search_parameters", {}) if probe else {}
    tag = f"{params.get('q', 'crawl')}_{params.get('geo') or 'WORLD'}".replace(" ", "-")
    out = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("exports") / f"crawl_{datetime.now():%Y%m%d}_{tag}"

    print("trend-service")
    export_trend(out / "trend-service")
    print("fb-ads-service")
    ads_by_url = export_fbads(out / "fb-ads-service")
    print("product-extractor-service")
    export_products(out / "product-extractor-service", ads_by_url)
    print("niche-research-service")
    export_niche(out / "niche-research-service")

    print("xlsx")
    for name in ["trend-service", "fb-ads-service", "product-extractor-service", "niche-research-service"]:
        to_xlsx(out / name)
    print(f"\n=> {out}")


if __name__ == "__main__":
    main()
