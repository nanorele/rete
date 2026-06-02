#!/usr/bin/env python3
# Regenerates the subset NotoSansCJK-Regular.otf from the full Noto Sans CJK SC.
#
#   1. Download the full font (OTF/CFF, ~16 MB):
#      https://github.com/notofonts/noto-cjk/raw/main/Sans/OTF/SimplifiedChinese/NotoSansCJKsc-Regular.otf
#   2. pip install fonttools brotli
#   3. python gen_cjk_subset.py NotoSansCJKsc-Regular.otf ttf/NotoSansCJK-Regular.otf
#   4. go run gen_compress.go   (rebuilds the embedded .br files)
#
# Coverage: GB2312 (Simplified Chinese), JIS X 0208 (Japanese), KS X 1001
# (Korean hangul + hanja), kana, and CJK punctuation / fullwidth forms.
# Runes outside this set fall back to a system CJK font at runtime.

import subprocess
import sys


def codepoints():
    cps = set()

    def add_charset(enc):
        for lead in range(0xA1, 0xFF):
            for trail in range(0xA1, 0xFF):
                try:
                    ch = bytes([lead, trail]).decode(enc)
                except Exception:
                    continue
                for c in ch:
                    cps.add(ord(c))

    add_charset("gb2312")
    add_charset("euc_jp")
    add_charset("euc_kr")

    for a, b in [
        (0x3000, 0x303F),  # CJK Symbols and Punctuation
        (0x3040, 0x309F),  # Hiragana
        (0x30A0, 0x30FF),  # Katakana
        (0x31F0, 0x31FF),  # Katakana Phonetic Extensions
        (0xFF00, 0xFFEF),  # Halfwidth and Fullwidth Forms
    ]:
        cps.update(range(a, b + 1))

    return sorted(c for c in cps if c >= 0x2000)


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: gen_cjk_subset.py <full-cjk.otf> <output.otf>")
    src, out = sys.argv[1], sys.argv[2]
    unicodes = ",".join("%04X" % c for c in codepoints())
    subprocess.run(
        [
            "pyftsubset",
            src,
            "--unicodes=" + unicodes,
            "--output-file=" + out,
            "--layout-features=*",
            "--drop-tables+=DSIG",
            "--name-IDs=1,2,3,4,6",
        ],
        check=True,
    )


if __name__ == "__main__":
    main()
