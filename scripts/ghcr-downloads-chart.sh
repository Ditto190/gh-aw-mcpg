#!/usr/bin/env bash
# ghcr-downloads-chart.sh
#
# Scrapes real download counts for ghcr.io/github/gh-aw-mcpg from the GitHub
# package page (using Playwright) and renders a two-page PDF chart:
#   Page 1 – downloads per tagged release (bar chart) + cumulative downloads
#   Page 2 – summary statistics table
#
# The GitHub Packages REST API does not expose download counts for container
# images; this script uses Playwright to scrape the figures that GitHub shows
# on the web UI instead.
#
# Requirements:
#   - python3 with playwright + matplotlib
#       pip3 install playwright matplotlib
#       python3 -m playwright install chromium
#
# Usage:
#   ./scripts/ghcr-downloads-chart.sh [output.pdf]

set -euo pipefail

ORG="github"
PACKAGE="gh-aw-mcpg"
OUTPUT="${1:-ghcr-downloads.pdf}"

# ── Preflight checks ──────────────────────────────────────────────────────────

check_cmd() {
  command -v "$1" &>/dev/null || { echo "Error: '$1' not found. $2" >&2; exit 1; }
}

check_cmd python3 "Install Python 3 from https://python.org"

for mod in playwright matplotlib; do
  python3 -c "import $mod" 2>/dev/null || {
    echo "Error: '$mod' not installed. Run: pip3 install $mod" >&2
    exit 1
  }
done

# ── Scrape download stats with Playwright + generate PDF ─────────────────────

echo "Scraping download stats for ghcr.io/${ORG}/${PACKAGE} ..."

export GHCR_ORG="$ORG"
export GHCR_PACKAGE="$PACKAGE"
export GHCR_OUTPUT="$OUTPUT"

python3 <<'EOF'
import os
import re
import json
import datetime
import itertools

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
from matplotlib.backends.backend_pdf import PdfPages
from playwright.sync_api import sync_playwright

org     = os.environ["GHCR_ORG"]
package = os.environ["GHCR_PACKAGE"]
output  = os.environ["GHCR_OUTPUT"]

PKG_URL      = f"https://github.com/orgs/{org}/packages/container/package/{package}"
VERSIONS_URL = f"https://github.com/{org}/{package}/pkgs/container/{package}/versions?filters[version_type]=tagged"

# ── Scrape ────────────────────────────────────────────────────────────────────

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)

    # Page 1: main package page → total downloads
    page = browser.new_page()
    page.goto(PKG_URL, wait_until="networkidle", timeout=30000)
    main_text = page.evaluate("document.body.innerText")

    # Page 2: all tagged versions → per-version download counts
    page.goto(VERSIONS_URL, wait_until="networkidle", timeout=30000)
    versions_text = page.evaluate("document.body.innerText")

    browser.close()

# ── Parse total downloads ─────────────────────────────────────────────────────

total_match = re.search(r'Total downloads\n([\d,]+K?)', main_text)
total_str   = total_match.group(1) if total_match else "N/A"

def parse_count(s):
    s = s.replace(",", "")
    if s.endswith("K"):
        return int(float(s[:-1]) * 1000)
    return int(s)

total_dl = parse_count(total_str) if total_str != "N/A" else 0

# ── Parse per-version download counts ────────────────────────────────────────
# Each entry on the "all tagged versions" page looks like:
#   <digest>\n<tag(s)>\nPublished X ago · Digest …\n<count>\nVersion downloads

entries = re.findall(
    r'([0-9a-f]{40})\n(.*?)\nPublished ([^\n]+)\n([\d,]+)\nVersion downloads',
    versions_text,
    re.DOTALL,
)

versions = []
for digest, tags_raw, published_raw, dl_str in entries:
    tag_list = [t.strip() for t in tags_raw.strip().splitlines() if t.strip()]
    versions.append({
        "digest":    digest,
        "tags":      tag_list,
        "published": published_raw.strip(),
        "downloads": parse_count(dl_str),
    })

# Sort newest → oldest (page order); reverse for oldest-first charts
versions_asc = list(reversed(versions))

# ── Print stats ───────────────────────────────────────────────────────────────

col = 30
sep = "─" * 65
print(f"\nghcr.io/{org}/{package}")
print(sep)
print(f"  {'Package':<{col}} ghcr.io/{org}/{package}")
print(f"  {'Total downloads (GitHub UI)':<{col}} {total_str}")
print(f"  {'Tagged releases tracked':<{col}} {len(versions)}")
print(sep)
print()
print(f"  {'Version':<14} {'Downloads':>12}  {'Cumulative':>12}  Published")
print(f"  {'─'*14} {'─'*12}  {'─'*12}  {'─'*20}")
running = 0
for v in versions_asc:
    running += v["downloads"]
    ver = next((t for t in v["tags"] if t.startswith("v")), v["tags"][0] if v["tags"] else "?")
    print(f"  {ver:<14} {v['downloads']:>12,}  {running:>12,}  {v['published']}")
print()

# ── Charts ────────────────────────────────────────────────────────────────────

labels    = [next((t for t in v["tags"] if t.startswith("v")), v["tags"][0] if v["tags"] else "?")
             for v in versions_asc]
downloads = [v["downloads"] for v in versions_asc]
cumulative = list(itertools.accumulate(downloads))
x = list(range(len(downloads)))

with PdfPages(output) as pdf:

    # ── Page 1: bar + cumulative ──────────────────────────────────────────────
    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(13, 11))
    fig.suptitle(
        f"ghcr.io/{org}/{package}  —  Container Image Downloads",
        fontsize=15, fontweight="bold", y=0.98,
    )

    # Top: per-version bar chart
    colors = plt.cm.tab20.colors
    bars = ax1.bar(x, downloads,
                   color=[colors[i % len(colors)] for i in x],
                   edgecolor="white", linewidth=0.5)
    ax1.set_title("Downloads per Tagged Release", pad=10)
    ax1.set_ylabel("Downloads")
    ax1.set_xticks(x)
    ax1.set_xticklabels(labels, rotation=45, ha="right", fontsize=9)
    ax1.yaxis.set_major_formatter(plt.FuncFormatter(lambda v, _: f"{int(v):,}"))
    ax1.grid(axis="y", linestyle="--", alpha=0.4)
    ax1.spines[["top", "right"]].set_visible(False)
    if downloads:
        y_pad = max(downloads) * 0.015
        for bar, val in zip(bars, downloads):
            if val > 0:
                ax1.text(bar.get_x() + bar.get_width() / 2,
                         bar.get_height() + y_pad,
                         f"{val:,}", ha="center", va="bottom", fontsize=7)

    # Bottom: cumulative line
    ax2.fill_between(x, cumulative, alpha=0.15, color="steelblue")
    ax2.plot(x, cumulative, color="steelblue", linewidth=2,
             marker="o", markersize=5)
    ax2.set_title("Cumulative Downloads", pad=10)
    ax2.set_ylabel("Total Downloads")
    ax2.set_xlabel("Release  (oldest → newest)")
    ax2.set_xticks(x)
    ax2.set_xticklabels(labels, rotation=45, ha="right", fontsize=9)
    ax2.yaxis.set_major_formatter(plt.FuncFormatter(lambda v, _: f"{int(v):,}"))
    ax2.grid(axis="y", linestyle="--", alpha=0.4)
    ax2.spines[["top", "right"]].set_visible(False)
    if cumulative:
        ax2.annotate(
            f"Tracked: {cumulative[-1]:,}",
            xy=(x[-1], cumulative[-1]),
            xytext=(-60, 12), textcoords="offset points",
            fontsize=9, color="steelblue",
            arrowprops=dict(arrowstyle="->", color="steelblue", lw=1),
        )

    plt.tight_layout(rect=[0, 0, 1, 0.96])
    pdf.savefig(fig)
    plt.close(fig)

    # ── Page 2: summary table ─────────────────────────────────────────────────
    fig2, ax = plt.subplots(figsize=(13, 8))
    ax.axis("off")
    fig2.suptitle("Summary Statistics", fontsize=14, fontweight="bold")

    top_idx   = downloads.index(max(downloads)) if downloads else 0
    top_label = labels[top_idx] if labels else "N/A"

    rows = [
        ["Metric", "Value"],
        ["Package",                    f"ghcr.io/{org}/{package}"],
        ["Total downloads (GitHub UI)", total_str],
        ["Tagged releases tracked",    str(len(versions))],
        ["Most downloaded release",    f"{top_label}  ({max(downloads, default=0):,} downloads)"],
        ["Tracked downloads (sum)",    f"{sum(downloads):,}"],
        ["Source",                     PKG_URL],
        ["Generated",                  datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")],
    ]

    tbl = ax.table(
        cellText=rows[1:],
        colLabels=rows[0],
        colWidths=[0.38, 0.55],
        loc="center",
        cellLoc="left",
    )
    tbl.auto_set_font_size(False)
    tbl.set_fontsize(12)
    tbl.scale(1, 2.4)

    for (row, col_idx), cell in tbl.get_celld().items():
        cell.set_edgecolor("#cccccc")
        if row == 0:
            cell.set_facecolor("#2c7bb6")
            cell.set_text_props(color="white", fontweight="bold")
        elif row % 2 == 0:
            cell.set_facecolor("#f0f4f8")
        else:
            cell.set_facecolor("white")

    plt.tight_layout(rect=[0, 0, 1, 0.94])
    pdf.savefig(fig2)
    plt.close(fig2)

    d = pdf.infodict()
    d["Title"]        = f"ghcr.io/{org}/{package} Downloads"
    d["Author"]       = "ghcr-downloads-chart.sh"
    d["Subject"]      = "Container image download statistics"
    d["CreationDate"] = datetime.datetime.now()

print(f"Saved: {output}  ({len(versions)} tagged releases, {sum(downloads):,} tracked downloads, {total_str} total)")
EOF
