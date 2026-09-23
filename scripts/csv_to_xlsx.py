#!/usr/bin/env python3
"""Gộp các file CSV trong exports/ thành một workbook .xlsx nhiều sheet.

Viết OOXML trực tiếp bằng zipfile + xml.etree vì môi trường này không có pip
(và do đó không có openpyxl/pandas). Dùng inline strings nên không cần
sharedStrings.xml.

Usage: python3 scripts/csv_to_xlsx.py [indir] [outfile]
"""
import csv
import re
import sys
import zipfile
from pathlib import Path
from xml.sax.saxutils import escape

INDIR = Path(sys.argv[1] if len(sys.argv) > 1 else "exports")
OUT = Path(sys.argv[2] if len(sys.argv) > 2 else INDIR / "crawl_data.xlsx")

# Thứ tự sheet theo luồng dữ liệu, không theo alphabet.
ORDER = ["jobs", "trend", "fbads", "products", "ai_results", "niche_sessions"]

# Excel từ chối mở file có control char trong inline string.
ILLEGAL = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1f]")
NUMERIC = re.compile(r"^-?\d+(\.\d+)?$")
MAXCELL = 32767  # giới hạn cứng của Excel cho một ô


def col_name(i):
    s = ""
    while i >= 0:
        s = chr(ord("A") + i % 26) + s
        i = i // 26 - 1
    return s


def cell(ref, value):
    if NUMERIC.match(value) and len(value) < 16:
        return f'<c r="{ref}"><v>{value}</v></c>'
    v = ILLEGAL.sub("", value)[:MAXCELL]
    return f'<c r="{ref}" t="inlineStr"><is><t xml:space="preserve">{escape(v)}</t></is></c>'


def sheet_xml(rows):
    out = [
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>',
        '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">',
        '<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" '
        'activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>',
        "<sheetData>",
    ]
    for r, row in enumerate(rows, start=1):
        out.append(f'<row r="{r}">')
        out += [cell(f"{col_name(c)}{r}", v) for c, v in enumerate(row)]
        out.append("</row>")
    out += ["</sheetData>", "</worksheet>"]
    return "".join(out)


files = [p for n in ORDER if (p := INDIR / f"{n}.csv").exists()]
files += sorted(p for p in INDIR.glob("*.csv") if p not in files)
if not files:
    sys.exit(f"khong tim thay CSV nao trong {INDIR}")

sheets = []
for p in files:
    with p.open(newline="", encoding="utf-8") as f:
        sheets.append((p.stem[:31], list(csv.reader(f))))

with zipfile.ZipFile(OUT, "w", zipfile.ZIP_DEFLATED) as z:
    z.writestr(
        "[Content_Types].xml",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        '<Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
        + "".join(
            f'<Override PartName="/xl/worksheets/sheet{i}.xml" '
            'ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>'
            for i in range(1, len(sheets) + 1)
        )
        + "</Types>",
    )
    z.writestr(
        "_rels/.rels",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>'
        "</Relationships>",
    )
    z.writestr(
        "xl/workbook.xml",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>'
        + "".join(
            f'<sheet name="{escape(n)}" sheetId="{i}" r:id="rId{i}"/>'
            for i, (n, _) in enumerate(sheets, start=1)
        )
        + "</sheets></workbook>",
    )
    z.writestr(
        "xl/_rels/workbook.xml.rels",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        + "".join(
            f'<Relationship Id="rId{i}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet{i}.xml"/>'
            for i in range(1, len(sheets) + 1)
        )
        + "</Relationships>",
    )
    for i, (_, rows) in enumerate(sheets, start=1):
        z.writestr(f"xl/worksheets/sheet{i}.xml", sheet_xml(rows))

print(f"{OUT}")
for n, rows in sheets:
    print(f"  sheet {n:<16} {max(len(rows) - 1, 0):>4} rows x {len(rows[0])} cols")
