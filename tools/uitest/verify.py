#!/usr/bin/env python3
"""Verify go-tracto UI screenshots against layout manifests and golden images.

For every <scene>.png + <scene>.layout.json produced by
`go test -tags screenshots ./internal/ui`, this script:

  1. checks region contours (bounds, non-overlap, real drawn edges),
  2. checks each region is non-blank (all widgets actually rendered),
  3. diffs against testdata/screenshots/golden/<scene>.png when present,

then writes an HTML report and exits non-zero on any failure.
"""

import argparse
import base64
import glob
import json
import os
import shutil
import sys

import cv2
import numpy as np

DEFAULT_DIR = os.path.join("internal", "ui", "testdata", "screenshots")

REGION_COLORS = {
    "titlebar": (66, 135, 245),
    "content": (120, 120, 120),
    "sidebar": (46, 204, 113),
    "main": (241, 196, 15),
    "overlay": (231, 76, 60),
}

# thresholds
MIN_REGION_STD = 3.0          # luminance stddev below this => blank panel
MIN_EDGE_DENSITY = 0.0008     # edge pixels / region area below this => no content
GOLDEN_PIXEL_DELTA = 12       # per-pixel abs diff counted as "changed"
GOLDEN_MAX_CHANGED = 0.005    # fraction of changed pixels above this => regression


def load_manifest(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def clamp_rect(rect, w, h):
    x0, y0, x1, y1 = rect
    return [max(0, x0), max(0, y0), min(w, x1), min(h, y1)]


def check_region_bounds(rect, w, h):
    x0, y0, x1, y1 = rect
    if x1 <= x0 or y1 <= y0:
        return f"degenerate rect {rect}"
    if x0 < 0 or y0 < 0 or x1 > w or y1 > h:
        return f"rect {rect} out of bounds {w}x{h}"
    return None


def check_non_blank(gray, rect):
    x0, y0, x1, y1 = clamp_rect(rect, gray.shape[1], gray.shape[0])
    crop = gray[y0:y1, x0:x1]
    if crop.size == 0:
        return "empty crop"
    if float(crop.std()) < MIN_REGION_STD:
        return f"blank (std={crop.std():.2f} < {MIN_REGION_STD})"
    return None


def check_edges(edges, rect):
    x0, y0, x1, y1 = clamp_rect(rect, edges.shape[1], edges.shape[0])
    crop = edges[y0:y1, x0:x1]
    if crop.size == 0:
        return "empty crop", 0.0
    density = float((crop > 0).sum()) / float(crop.size)
    if density < MIN_EDGE_DENSITY:
        return f"no content edges (density={density:.5f} < {MIN_EDGE_DENSITY})", density
    return None, density


def overlaps(a, b):
    ax0, ay0, ax1, ay1 = a
    bx0, by0, bx1, by1 = b
    ix = max(0, min(ax1, bx1) - max(ax0, bx0))
    iy = max(0, min(ay1, by1) - max(ay0, by0))
    return ix * iy


def check_structure(regions):
    by_name = {r["name"]: r["rect"] for r in regions}
    msgs = []
    if "sidebar" in by_name and "main" in by_name:
        if by_name["main"][0] < by_name["sidebar"][2]:
            msgs.append("sidebar and main overlap horizontally")
    if "titlebar" in by_name and "content" in by_name:
        if by_name["content"][1] < by_name["titlebar"][3]:
            msgs.append("titlebar and content overlap vertically")
    return msgs


def annotate(img, regions, out_path):
    canvas = img.copy()
    for r in regions:
        x0, y0, x1, y1 = r["rect"]
        col = REGION_COLORS.get(r["name"], (200, 200, 200))
        bgr = (col[2], col[1], col[0])
        cv2.rectangle(canvas, (x0, y0), (x1 - 1, y1 - 1), bgr, 2)
        cv2.putText(canvas, r["name"], (x0 + 4, y0 + 18),
                    cv2.FONT_HERSHEY_SIMPLEX, 0.5, bgr, 1, cv2.LINE_AA)
    cv2.imwrite(out_path, canvas)


def golden_diff(img, golden_path, out_path):
    golden = cv2.imread(golden_path, cv2.IMREAD_COLOR)
    if golden is None:
        return None, "golden unreadable"
    if golden.shape != img.shape:
        return None, f"size mismatch {golden.shape} vs {img.shape}"
    delta = cv2.absdiff(img, golden).max(axis=2)
    changed = delta > GOLDEN_PIXEL_DELTA
    frac = float(changed.sum()) / float(changed.size)
    heat = cv2.applyColorMap((delta.clip(0, 255)).astype(np.uint8), cv2.COLORMAP_JET)
    heat[~changed] = (40, 40, 40)
    cv2.imwrite(out_path, heat)
    msg = None
    if frac > GOLDEN_MAX_CHANGED:
        msg = f"changed {frac*100:.2f}% > {GOLDEN_MAX_CHANGED*100:.2f}%"
    return frac, msg


def verify_scene(png_path, manifest_path, golden_dir, annot_dir, diff_dir):
    man = load_manifest(manifest_path)
    img = cv2.imread(png_path, cv2.IMREAD_COLOR)
    result = {"scene": man["scene"], "png": png_path, "messages": [], "ok": True,
              "annotated": None, "diff": None, "golden_frac": None}
    if img is None:
        result["ok"] = False
        result["messages"].append("png unreadable")
        return result

    h, w = img.shape[:2]
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    edges = cv2.Canny(gray, 40, 120)

    for r in man["regions"]:
        name, rect = r["name"], r["rect"]
        for msg in [check_region_bounds(rect, w, h),
                    check_non_blank(gray, rect),
                    check_edges(edges, rect)[0]]:
            if msg:
                result["ok"] = False
                result["messages"].append(f"{name}: {msg}")

    for msg in check_structure(man["regions"]):
        result["ok"] = False
        result["messages"].append(msg)

    annot_path = os.path.join(annot_dir, man["scene"] + ".png")
    annotate(img, man["regions"], annot_path)
    result["annotated"] = annot_path

    golden_path = os.path.join(golden_dir, man["scene"] + ".png")
    if os.path.isfile(golden_path):
        diff_path = os.path.join(diff_dir, man["scene"] + ".png")
        frac, msg = golden_diff(img, golden_path, diff_path)
        result["golden_frac"] = frac
        result["diff"] = diff_path
        if msg:
            result["ok"] = False
            result["messages"].append(f"golden: {msg}")
    else:
        result["messages"].append("no golden (baseline missing)")

    return result


def data_uri(path):
    if not path or not os.path.isfile(path):
        return ""
    with open(path, "rb") as f:
        b = base64.b64encode(f.read()).decode("ascii")
    return "data:image/png;base64," + b


def write_report(results, out_path):
    rows = []
    for r in results:
        status = "PASS" if r["ok"] else "FAIL"
        color = "#1a7f37" if r["ok"] else "#cf222e"
        frac = "" if r["golden_frac"] is None else f"{r['golden_frac']*100:.3f}% changed"
        msgs = "<br>".join(r["messages"]) or "&mdash;"
        imgs = [("screenshot", r["png"]), ("layout", r["annotated"]), ("golden diff", r["diff"])]
        cells = "".join(
            f'<div class="cell"><div class="cap">{cap}</div>'
            f'<img src="{data_uri(p)}"></div>'
            for cap, p in imgs if p
        )
        rows.append(
            f'<section><h2 style="color:{color}">{r["scene"]} &mdash; {status}</h2>'
            f'<p class="meta">{frac}</p><p class="msg">{msgs}</p>'
            f'<div class="imgs">{cells}</div></section>'
        )
    failed = sum(1 for r in results if not r["ok"])
    head = (
        "<!doctype html><meta charset='utf-8'><title>UI screenshot report</title>"
        "<style>body{font:14px system-ui;background:#0d1117;color:#c9d1d9;margin:24px}"
        "h1{font-size:20px}section{border:1px solid #30363d;border-radius:8px;padding:12px;margin:16px 0}"
        "h2{font-size:16px;margin:0 0 6px}.meta{color:#8b949e;margin:2px 0}.msg{color:#d29922;margin:2px 0}"
        ".imgs{display:flex;gap:12px;flex-wrap:wrap}.cell{flex:1 1 380px}.cap{color:#8b949e;font-size:12px;margin-bottom:4px}"
        "img{width:100%;border:1px solid #30363d;border-radius:6px}</style>"
        f"<h1>UI screenshot report &mdash; {len(results)} scenes, {failed} failed</h1>"
    )
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(head + "".join(rows))


def update_golden(dir_, golden_dir):
    os.makedirs(golden_dir, exist_ok=True)
    n = 0
    for png in glob.glob(os.path.join(dir_, "*.png")):
        if os.path.basename(os.path.dirname(png)) == "golden":
            continue
        shutil.copy2(png, os.path.join(golden_dir, os.path.basename(png)))
        n += 1
    print(f"updated {n} golden images in {golden_dir}")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--dir", default=DEFAULT_DIR, help="screenshots directory")
    ap.add_argument("--golden", default=None, help="golden directory (default <dir>/golden)")
    ap.add_argument("--report", default=None, help="HTML report path (default <dir>/report.html)")
    ap.add_argument("--update-golden", action="store_true", help="copy current PNGs to golden/ and exit")
    args = ap.parse_args()

    dir_ = args.dir
    golden_dir = args.golden or os.path.join(dir_, "golden")
    report_path = args.report or os.path.join(dir_, "report.html")

    if not os.path.isdir(dir_):
        print(f"error: directory not found: {dir_}", file=sys.stderr)
        return 2

    if args.update_golden:
        update_golden(dir_, golden_dir)
        return 0

    annot_dir = os.path.join(dir_, "annotated")
    diff_dir = os.path.join(dir_, "diff")
    os.makedirs(annot_dir, exist_ok=True)
    os.makedirs(diff_dir, exist_ok=True)

    manifests = sorted(glob.glob(os.path.join(dir_, "*.layout.json")))
    if not manifests:
        print(f"error: no *.layout.json in {dir_} (run the screenshot test first)", file=sys.stderr)
        return 2

    results = []
    for man in manifests:
        png = man[: -len(".layout.json")] + ".png"
        if not os.path.isfile(png):
            results.append({"scene": os.path.basename(png), "png": png, "ok": False,
                            "messages": ["png missing"], "annotated": None, "diff": None,
                            "golden_frac": None})
            continue
        results.append(verify_scene(png, man, golden_dir, annot_dir, diff_dir))

    write_report(results, report_path)

    failed = [r for r in results if not r["ok"]]
    for r in results:
        status = "PASS" if r["ok"] else "FAIL"
        extra = "" if r["ok"] else "  :: " + "; ".join(r["messages"])
        print(f"[{status}] {r['scene']}{extra}")
    print(f"\n{len(results)} scenes, {len(failed)} failed -> {report_path}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
