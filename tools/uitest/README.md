# UI screenshot integration tests

Render every UI mode of the live app to a PNG, then verify region layout,
that all widgets actually drew, and that nothing regressed against a golden baseline.

## Layers

1. **Render (Go).** `internal/ui/screenshot_test.go` (build tag `screenshots`) builds an
   `AppUI` per scene and rasterizes the full UI through `gpu/headless`. Each scene is rendered at
   every size in `shotSizes` (currently `1280x800` and the app's `480x360` minimum), so layout is
   checked at both a roomy and a cramped extent. Output per scene/size:
   `internal/ui/testdata/screenshots/<scene>_<W>x<H>.png` and `<scene>_<W>x<H>.layout.json`
   (expected region rects: `titlebar`, `content`, `sidebar`, `main`).
2. **Verify (Python).** `verify.py` checks region bounds / non-overlap / drawn edges, checks each
   region is non-blank, diffs against `golden/<scene>.png`, and writes `report.html`.

## Run

```sh
# 1. render all scenes (needs a GPU backend; tests Skip if none is available)
go test -tags screenshots ./internal/ui -run TestScreenshots -count=1

# 2. one-time: establish golden baselines after eyeballing the PNGs
python tools/uitest/verify.py --update-golden

# 3. verify (exit code != 0 on any failure)
pip install numpy opencv-python   # or: pip install -r tools/uitest/requirements.txt
python tools/uitest/verify.py
# open internal/ui/testdata/screenshots/report.html
```

`go test` without `-tags screenshots` ignores all of this and needs no GPU.

> The repo's root `.gitignore` matches `*.txt`, `*.json`, `*.png` and the whole
> `internal/ui/testdata` tree, so `requirements.txt`, the rendered PNGs, the `*.layout.json`
> manifests and any goldens are **not committed** — they are regenerated each run. CI installs
> the Python deps inline for this reason. Committing baseline goldens (below) first requires
> un-ignoring those paths in the root `.gitignore`.

## Headless / CI

`gpu/headless` has no software rasterizer; it needs a GPU backend:

- **Windows:** D3D11 (works locally; D3D11 WARP for a software adapter).
- **Linux CI:** Mesa **llvmpipe** gives a software EGL context (surfaceless, no display) —
  `LIBGL_ALWAYS_SOFTWARE=1`, `GALLIUM_DRIVER=llvmpipe`, `EGL_PLATFORM=surfaceless`.

When `headless.NewWindow` returns an error the test calls `t.Skip`. The CI job guards against a
false green: if every scene skipped, no PNGs are produced and `verify.py` exits non-zero
("no \*.layout.json").

The `ui-screenshots` job in `.github/workflows/ci.yml` runs on the self-hosted Ubuntu 24 runner:
it installs `libgl1-mesa-dri` + `mesa-vulkan-drivers`, renders via llvmpipe, verifies, and uploads
the PNGs + `report.html` as an artifact.

### Goldens are per-backend

Pixels from llvmpipe (Linux) and D3D11 (Windows) differ (rasterizer, AA, font hinting), so a golden
captured on one backend won't pixel-match the other. The committed `golden/` is the local
(Windows) baseline. CI runs only the backend-tolerant checks (contours + non-blank) by pointing
`--golden` at `golden-linux/`, which is empty by default.

To enable pixel-diff regression on CI, generate Linux baselines on the runner and commit them:

```sh
# on the Ubuntu 24 runner, after rendering:
python tools/uitest/verify.py --update-golden --golden internal/ui/testdata/screenshots/golden-linux
```

(or download the `ui-screenshots` artifact and commit its PNGs into `golden-linux/`). After that,
drop the `--golden golden-linux` override or keep it — the CI step will start diffing automatically.

## Adding a scene

Append to `sceneList()` in `internal/ui/screenshot_test.go`: a name and a `func(*AppUI)` that sets the
state for that mode (e.g. `ui.SidebarSection = "mitm"`, an overlay flag, `ui.SettingsOpen = true`).
Seed data (one collection + one environment) is applied to every scene by `seedTestData`.

## Thresholds

Tune the constants at the top of `verify.py`: `MIN_REGION_STD`, `MIN_EDGE_DENSITY`,
`GOLDEN_PIXEL_DELTA`, `GOLDEN_MAX_CHANGED`.
