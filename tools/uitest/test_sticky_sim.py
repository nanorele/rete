#!/usr/bin/env python3
"""Real, runnable tests for the collections sticky-scroll band.

These mirror the Go ``internal/ui/sticky_lag_test.go`` invariants in Python on
top of ``sticky_sim.py`` (a faithful port of the gio List + ``stickyHeaders``
logic), and add the one invariant the Go test is structurally unable to catch:
the *forward lurch* at the first node of a second/subsequent sticky folder.

The module models BOTH behaviours:
  * ``fixed=False`` -- the shipped Go algorithm (reproduces the jerk).
  * ``fixed=True``  -- the proposed fix (descent vs sibling-swap; swap keeps a
    full, constant reserve and slides the inner row -> no content jump).

Run directly (no pytest needed):

    python tools/uitest/test_sticky_sim.py

or under pytest:

    pytest tools/uitest/test_sticky_sim.py

Invariants (per frame, while scrolling down by ``delta`` px):
  1. no lag      -- the solid band == the true ancestor chain (+ top on descent).
  2. no occlusion-- reserve <= band_h, and reserve == band_h when not entering.
  3. no backward -- effective (= absScroll - reserve) never DECREASES.
  4. no lurch    -- effective never INCREASES by more than ``delta`` (+tol):
                    a sudden forward jump is just as visible a jerk as a
                    backward one.

Reference behaviour: VS Code "sticky scroll". When the topmost folder's scope
ends and a sibling folder begins, the leaving header slides up and out while
the entering header slides up and in; the list content keeps moving at a
constant rate. No row of content ever jumps.
"""

from __future__ import annotations

import sys
from typing import List, Tuple

from sticky_sim import Frame, Node, Sim, deep_chain, sibling_folders

DELTA = 6          # px per scroll step (matches the Go lag test)
TOL = 2            # px rounding tolerance (matches the Go lag test's +-2)


# --- expected band oracle (mirrors expectedBand() in sticky_lag_test.go) ------

def expected_solid(snap: List[Node], first: int, mode: str) -> List[str]:
    top = snap[first]
    names = []
    p = top.parent
    while p is not None:
        names.append(p.name)
        p = p.parent
    names.reverse()
    if mode == "descent":
        names.append(top.name)
    return names


# --- invariant checkers: return list of human-readable violations -------------

def check_no_lag(sim_builder, fixed: bool, frames: List[Frame]) -> List[str]:
    bad = []
    for f in frames:
        scratch = Sim(sim_builder(), fixed=fixed)
        want = expected_solid(scratch.snap, f.first, f.mode)
        if f.band != want:
            bad.append(f"step {f.step}: solid band {f.band} != expected {want} "
                       f"(mode={f.mode})")
    return bad


def check_no_occlusion(frames: List[Frame]) -> List[str]:
    bad = []
    for f in frames:
        if f.reserve > f.band_h:
            bad.append(f"step {f.step}: reserve {f.reserve} > band_h {f.band_h}")
        if not f.entering and f.reserve != f.band_h:
            bad.append(f"step {f.step}: not entering but reserve {f.reserve} "
                       f"!= band_h {f.band_h} (list occluded)")
    return bad


def check_no_backward(frames: List[Frame]) -> List[str]:
    bad = []
    prev = frames[0].effective
    for f in frames[1:]:
        if f.effective < prev - TOL:
            bad.append(f"step {f.step}: content scrolled BACKWARDS "
                       f"effective {prev} -> {f.effective}")
        prev = f.effective
    return bad


def check_no_lurch(frames: List[Frame], delta: int = DELTA) -> List[str]:
    bad = []
    prev = frames[0].effective
    for f in frames[1:]:
        d = f.effective - prev
        if d > delta + TOL:
            bad.append(f"step {f.step}: content LURCHED FORWARD "
                       f"d_effective {d:+d} (> scroll {delta}); "
                       f"band={f.band} mode={f.mode} reserve={f.reserve}")
        prev = f.effective
    return bad


def _all_checks(builder, fixed: bool, steps: int) -> Tuple[Sim, List[Frame], dict]:
    s = Sim(builder(), fixed=fixed)
    frames = s.scroll(steps, DELTA)
    return s, frames, {
        "lag": check_no_lag(builder, fixed, frames),
        "occlusion": check_no_occlusion(frames),
        "backward": check_no_backward(frames),
        "lurch": check_no_lurch(frames),
    }


# --- pytest-collectable tests -------------------------------------------------

def test_buggy_sibling_folders_reproduces_jerk():
    """REGRESSION REPRODUCER: the shipped algorithm lurches at each sibling swap.

    The three Go-test invariants (lag/occlusion/backward) all PASS -- which is
    exactly why the shipped Go test never caught it -- but the content lurches
    forward by one band row (24px) on top of the 6px scroll at the fldA->fldB
    and fldB->fldC boundaries.
    """
    _, _, c = _all_checks(sibling_folders, fixed=False, steps=40)
    assert c["lag"] == []
    assert c["occlusion"] == []
    assert c["backward"] == []                 # forward lurch is invisible here
    assert c["lurch"], "expected the shipped algorithm to lurch -- it did not"
    assert all("d_effective +30" in v for v in c["lurch"]), c["lurch"]


def test_fixed_sibling_folders_is_smooth():
    """THE FIX: sibling swaps keep a full, constant reserve -> no lurch."""
    _, frames, c = _all_checks(sibling_folders, fixed=True, steps=40)
    assert c["lag"] == [], c["lag"]
    assert c["occlusion"] == [], c["occlusion"]
    assert c["backward"] == [], c["backward"]
    assert c["lurch"] == [], c["lurch"]        # <- the jerk is gone
    # And the swap really did engage (push-out) at the boundaries.
    swaps = [f for f in frames if f.mode == "swap"]
    assert swaps, "fix never classified any frame as a sibling swap"
    assert any(f.leaving == "fldA" and f.entering_name == "fldB" for f in swaps)
    assert any(f.leaving == "fldB" and f.entering_name == "fldC" for f in swaps)


def test_fixed_deep_chain_descent_unchanged():
    """The fix must not disturb pure descent (the Go lag test's tree)."""
    _, _, c = _all_checks(deep_chain, fixed=True, steps=60)
    assert c["lag"] == [], c["lag"]
    assert c["occlusion"] == [], c["occlusion"]
    assert c["backward"] == [], c["backward"]
    assert c["lurch"] == [], c["lurch"]


def test_buggy_deep_chain_is_smooth():
    """Sanity: the bug is swap-specific -- pure descent is smooth even unfixed."""
    _, _, c = _all_checks(deep_chain, fixed=False, steps=60)
    assert c["lurch"] == [], c["lurch"]


# --- standalone runner (no pytest required) -----------------------------------

def _run():
    tests = [
        test_buggy_sibling_folders_reproduces_jerk,
        test_fixed_sibling_folders_is_smooth,
        test_fixed_deep_chain_descent_unchanged,
        test_buggy_deep_chain_is_smooth,
    ]
    fails = 0
    for t in tests:
        try:
            t()
            print(f"PASS  {t.__name__}")
        except AssertionError as e:
            fails += 1
            print(f"FAIL  {t.__name__}\n      {e}")

    print("\n--- jerk: shipped vs fixed (sibling folders) ---")
    _, _, before = _all_checks(sibling_folders, fixed=False, steps=40)
    _, _, after = _all_checks(sibling_folders, fixed=True, steps=40)
    print("  BEFORE fix:")
    for v in before["lurch"]:
        print("    " + v)
    print(f"  AFTER fix: {after['lurch'] or 'no lurch -- smooth, matches VS Code'}")
    return fails


if __name__ == "__main__":
    sys.exit(1 if _run() else 0)
