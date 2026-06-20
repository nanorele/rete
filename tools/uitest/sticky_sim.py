#!/usr/bin/env python3
"""Headless Python model of the collections sticky-scroll band.

This is a faithful port of the load-bearing pieces of the Go implementation so
the sticky behaviour can be exercised (and its bugs reproduced) without the
Gio/GPU stack. See ``sticky.md`` for the full design narrative. The three
pieces modelled here, and where they live in Go:

  * ``List.layout`` advance loop  -> gio ``layout/list.go`` (how a scroll delta
    redistributes into ``Position.First`` / ``Position.Offset``).
  * ``stickyHeaders``             -> ``internal/ui/sidebar/sidebar.go`` (which
    ancestors get pinned, the "entering" detection, and the smooth reserve).
  * ``colsArea``                  -> ``internal/ui/sidebar/sidebar.go`` (the
    list is offset down by ``reserve``; here we only need the reserve value).

Only the vertical, forward/backward single-pointer scroll path is modelled —
that is all the sticky band depends on. Drag, fling, touch and gap are omitted.

The quantity that matters for "does it jerk" is the on-screen position of a
fixed content anchor. With the list offset down by ``reserve`` and scrolled by
``absScroll`` pixels, a fixed row sits at ``screen_y = const - absScroll +
reserve``. So ``effective = absScroll - reserve`` is proportional to the
*negative* of that screen position. Smooth scrolling => ``effective`` moves by
roughly the scroll delta each frame; a JERK is a sudden ``effective`` jump
(forward or backward) that is not justified by the scroll the user applied.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Optional


# --- tree model (mirror of collections.CollectionNode, only sticky-relevant fields) ---

@dataclass
class Node:
    name: str
    is_folder: bool = False
    expanded: bool = False
    # list_h: height of this node's row *in the scrolling list* (px).
    # band_h: height of this node's row when painted *in the sticky band* (px).
    # In the real app band rows are always single-line (MaxLines 1) while list
    # rows allow 2 lines, so band_h can be <= list_h. Default them equal.
    list_h: int = 24
    band_h: int = 24
    depth: int = 0
    parent: Optional["Node"] = None
    children: List["Node"] = field(default_factory=list)


def assign(root: Node, depth: int = 0, parent: Optional[Node] = None) -> None:
    root.depth = depth
    root.parent = parent
    for c in root.children:
        assign(c, depth + 1, root)


def flatten(root: Node) -> List[Node]:
    """Pre-order flatten of the expanded tree == VisibleCols."""
    out: List[Node] = [root]
    if root.expanded:
        for c in root.children:
            out.extend(flatten(c))
    return out


# --- gio List Position (mirror of layout.Position, vertical only) ---

@dataclass
class Position:
    first: int = 0
    offset: int = 0


def list_advance(pos: Position, heights: List[int], delta: int) -> None:
    """Apply a scroll ``delta`` (px) then redistribute into first/offset.

    Mirrors gio ``List.update`` (``Offset += delta``) followed by the
    ``List.layout`` forward/backward advance loop. The end/start clamps that
    require the full viewport size are simplified to the bounds we exercise.
    """
    n = len(heights)
    pos.offset += delta
    # forward: while the current first row is fully scrolled past, drop it.
    while pos.first < n - 1 and pos.offset >= heights[pos.first]:
        pos.offset -= heights[pos.first]
        pos.first += 1
    # backward: negative offset pulls the previous row back into view.
    while pos.first > 0 and pos.offset < 0:
        pos.first -= 1
        pos.offset += heights[pos.first]
    if pos.first == 0 and pos.offset < 0:
        pos.offset = 0


# --- stickyHeaders (mirror of sidebar.go stickyHeaders) ---

BORDER = 1  # the 1px bottom border flex child of the band


@dataclass
class Band:
    # ``names``: the solid, fully-pinned ancestor rows (top-to-bottom).
    # ``leaving``/``entering_name``: during a sibling swap, the inner band slot
    #   shows the leaving folder sliding up and out while the entering folder's
    #   own list row rises to dock -- a 2-row strip scrolling through a 1-row
    #   window (VS Code "push-out"). ``None`` outside a swap.
    names: List[str]
    band_h: int
    reserve: int
    entering: bool            # True while a bottom row is in transition
    mode: str = "none"        # "none" | "descent" | "swap"
    leaving: Optional[str] = None
    entering_name: Optional[str] = None


def _ancestors(top: Node) -> List[Node]:
    anc: List[Node] = []
    p = top.parent
    while p is not None:
        anc.append(p)
        p = p.parent
    anc.reverse()  # outermost first
    return anc


def sticky_headers(snap: List[Node], pos: Position, viewport_h: int,
                   max_rows_enabled: bool = True, fixed: bool = False) -> Band:
    """Compute the pinned ancestor band and the smooth reserve for this frame.

    ``fixed=False`` reproduces the shipped Go behaviour (the jerk). ``fixed=True``
    is the proposed fix: a folder that becomes the top row is only treated as a
    grow-the-band "descent" (slide-in / freeze) when the row just above it is its
    own parent. When instead it *replaces* a sibling-level ancestor (the previous
    scope just ended), it is a "swap": the band keeps its full reserve and only
    the inner row's content slides (leaving folder out, entering folder in), so
    the list content never jumps.
    """
    first = pos.first
    if first < 0 or first >= len(snap):
        return Band([], 0, 0, False)
    top = snap[first]

    anc = _ancestors(top)

    entering = ((top.is_folder or top.depth == 0) and top.expanded and
                first + 1 < len(snap) and snap[first + 1].parent is top)

    mode = "none"
    leaving = None
    if entering:
        if not fixed:
            mode = "descent"          # shipped code: always slide-in
        else:
            # Descent iff the row above us is our own parent (or we are the very
            # first row). Otherwise it is a sibling swap.
            descent = first == 0 or snap[first - 1] is top.parent
            if descent:
                mode = "descent"
            else:
                # Find the leaving sibling: climb the previous row up by parent
                # until it shares our parent (robust if depth is stale).
                o = snap[first - 1]
                while o is not None and o.parent is not top.parent:
                    o = o.parent
                if o is not None and o is not top and o.parent is top.parent:
                    mode = "swap"
                    leaving = o
                else:
                    mode = "descent"  # not a clean sibling swap -> safe fallback

    if mode == "descent":
        anc = anc + [top]
    if not anc and mode != "swap":
        return Band([], 0, 0, False)

    # maxRows cap: keep the innermost ancestors (mirror of the gtx.Dp(24) logic).
    if max_rows_enabled:
        approx_h = 24
        max_rows = viewport_h // approx_h - 2
        if max_rows >= 1 and len(anc) > max_rows:
            anc = anc[len(anc) - max_rows:]

    # solid rows + (one transition slot during a swap).
    band_rows = len(anc) + (1 if mode == "swap" else 0)
    band_h = sum(a.band_h for a in anc) + (top.band_h if mode == "swap" else 0) + BORDER

    reserve = band_h
    if mode == "descent":
        # bandRowH = (band_h - border) / len(anc)  -- integer division, the
        # *average* band-row height (== uniform band-row height when equal).
        band_row_h = (band_h - BORDER) // len(anc)
        off = pos.offset
        if off > band_row_h:
            off = band_row_h
        if off < 0:
            off = 0
        reserve = band_h - band_row_h + off
    # mode == "swap": reserve stays full (band_h) and constant -> no content jump;
    # the inner slot slides leaving->entering as Offset grows.
    if reserve < 0:
        reserve = 0

    return Band(
        names=[a.name for a in anc],
        band_h=band_h,
        reserve=reserve,
        entering=(mode != "none"),
        mode=mode,
        leaving=leaving.name if leaving is not None else None,
        entering_name=top.name if mode == "swap" else None,
    )


# --- frame driver -----------------------------------------------------------

@dataclass
class Frame:
    step: int
    first: int
    offset: int
    abs_scroll: int
    reserve: int
    band_h: int
    entering: bool
    band: List[str]
    effective: int
    mode: str = "none"
    leaving: Optional[str] = None
    entering_name: Optional[str] = None


class Sim:
    """Drives scroll frames over a flattened tree, recording each frame."""

    def __init__(self, root: Node, viewport_h: int = 320,
                 max_rows_enabled: bool = True, fixed: bool = False):
        assign(root)
        self.root = root
        self.snap = flatten(root)
        self.heights = [n.list_h for n in self.snap]
        self.viewport_h = viewport_h
        self.max_rows_enabled = max_rows_enabled
        self.fixed = fixed
        self.pos = Position()

    def abs_scroll(self) -> int:
        p = self.pos.offset
        for i in range(min(self.pos.first, len(self.heights))):
            p += self.heights[i]
        return p

    def frame(self, step: int, delta: int) -> Frame:
        list_advance(self.pos, self.heights, delta)
        band = sticky_headers(self.snap, self.pos, self.viewport_h,
                              self.max_rows_enabled, fixed=self.fixed)
        a = self.abs_scroll()
        return Frame(step=step, first=self.pos.first, offset=self.pos.offset,
                     abs_scroll=a, reserve=band.reserve, band_h=band.band_h,
                     entering=band.entering, band=band.names,
                     effective=a - band.reserve, mode=band.mode,
                     leaving=band.leaving, entering_name=band.entering_name)

    def scroll(self, steps: int, delta: int = 6) -> List[Frame]:
        frames = [self.frame(0, 0)]
        for s in range(1, steps + 1):
            frames.append(self.frame(s, delta))
        return frames


# --- tree builders used by tests / trace ------------------------------------

def deep_chain() -> Node:
    """root -> A -> B -> C -> D -> 40 requests (the Go lag test's tree).

    Every visible folder is the first child of its parent, so every folder is
    'entering' in turn -- this tree never exercises a leaf->sibling-folder
    boundary, which is exactly why the Go test misses the jerk.
    """
    root = Node("root", is_folder=True, expanded=True)
    cur = root
    for nm in ("A", "B", "C", "D"):
        f = Node(nm, is_folder=True, expanded=True)
        cur.children.append(f)
        cur = f
    for i in range(40):
        cur.children.append(Node(f"req-{i}"))
    return root


def sibling_folders() -> Node:
    """root -> [fldA -> a0,a1 ; fldB -> b0,b1 ; fldC -> c0,c1].

    Scrolling past fldA's last child (a1) into fldB makes the band's *second*
    row switch fldA -> fldB. fldB is a fresh 'entering' folder, so its reserve
    restarts from band_h - band_row_h -- a discrete drop of one band row vs the
    full reserve held a frame earlier. That is the reported jerk at the first
    node of the (second/subsequent) folder.
    """
    root = Node("root", is_folder=True, expanded=True)
    for fn in ("fldA", "fldB", "fldC"):
        f = Node(fn, is_folder=True, expanded=True)
        for i in range(2):
            f.children.append(Node(f"{fn[-1].lower()}{i}"))
        root.children.append(f)
    return root


def _print_trace(title: str, frames: List[Frame]) -> None:
    print(f"\n=== {title} ===")
    print(f"{'st':>3} {'first':>5} {'off':>4} {'absS':>5} {'rsrv':>5} "
          f"{'bandH':>5} {'mode':>7} {'dEff':>5}  band")
    prev = None
    for f in frames:
        deff = "" if prev is None else f"{f.effective - prev:+d}"
        band = list(f.band)
        if f.mode == "swap":
            band = band + [f"{f.leaving}>>{f.entering_name}"]
        print(f"{f.step:>3} {f.first:>5} {f.offset:>4} {f.abs_scroll:>5} "
              f"{f.reserve:>5} {f.band_h:>5} {f.mode:>7} "
              f"{deff:>5}  {band}")
        prev = f.effective


if __name__ == "__main__":
    _print_trace("sibling folders -- BEFORE fix (THE JERK)",
                 Sim(sibling_folders(), fixed=False).scroll(40, 6))
    _print_trace("sibling folders -- AFTER fix (push-out, smooth)",
                 Sim(sibling_folders(), fixed=True).scroll(40, 6))
    _print_trace("deep chain -- AFTER fix (descent unchanged, still smooth)",
                 Sim(deep_chain(), fixed=True).scroll(60, 6))
