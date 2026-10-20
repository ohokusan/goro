# Poring Cash Mounts (Novice)

Goro recognizes `EFST_ALL_RIDING` (613) from the existing `0x0196`
20080910 status packet. Activation and removal change the rendered body
immediately for the local character and visible remote actors. This is separate
from the Peco Riding option; no server, packet-version, or database changes are
needed. An already-mounted remote actor is recognized when the server sends its
active EFST on entry into view. Speed is never used to infer mounting.

Only job 0 selects a Poring body. Place these pairs beneath the configured
client data directory, keeping their names:

- `data/sprite/custom/poring_novice_male.spr`
- `data/sprite/custom/poring_novice_male.act`
- `data/sprite/custom/poring_novice_female.spr`
- `data/sprite/custom/poring_novice_female.act`

Original sprites and GRFs remain untouched. Assets are not included in this
repository or in the executable.

## Appearance and fallback

The custom body uses its SPR's default palette. Job, hairstyle, hair dye,
headgear, clothing dye, and equipment values remain unchanged. The head follows
the custom body's ACT attachment point; headgear follows the original head's
attachment point. Ordinary Novice IMF offsets are bypassed for an accepted
custom body. Equipment overlays retain their existing fallback rendering.

On dismount, the ordinary body, palette and IMF are loaded from the current
appearance, including any look changes received while mounted. Remote cached
views include cash-mount state in their key, so mounted and ordinary bodies do
not share rendered billboards. Parsed resources remain immutable and shared.

Missing/unreadable files, parse errors, mismatched SPR references, non-finite
scales, missing idle/walk directions, missing head anchors, or empty required
body frames fall back to the ordinary body and its clothing palette. The
loader reports `cash-mount-fallback` in its sprite resource diagnostic. A
fallback view can be cached until it is reloaded or the map changes; installing
or replacing assets while the game is running is not a supported hot-reload
operation.

## Experimental assets inspected locally

The supplied male and female files both parsed as ACT 2.3 and SPR 2.0. Each
contains 104 actions and 34 indexed sprite frames. All eight idle actions have
one frame each; all eight walk actions have four frames each, with a 155 ms ACT
delay. Required frames have attachment points and valid SPR references.
Single-frame idle is valid, but these assets do not contain an animated idle
cycle. Most other actions are single-frame poses; this change does not create
missing attack, sitting, or death animations.

The first head-only repair was insufficient. Live screenshots showed incorrect
rear head placement and a Poring body floating above its shadow and health bar.
The conversion used one body Y offset (-54) for every direction in a 136x108
canvas. Rear artwork has a much larger transparent margin below the principal
body than the front artwork. The trailing decorative mini-Porings made a
whole-sprite bounding box an incorrect ground-contact measurement.

The revised repair does three things:

1. Aligns the principal Poring body's contact point to Y=-2 relative to the
   ground origin, close to the male mounted Knight's rear foot point (~-1.5).
   Body Y adjustments are 2, 2, 2, 18, 24, 18, 2, 2 source pixels by direction.
   The shadow, health bar and world position retain their existing coordinates.
2. Derives native head attachment coordinates from the ordinary sex-specific
   Novice body's ACT reference and registered neck/collar artwork. Rear head
   ACT attachment points are not anatomical neck centers; the previous repair
   estimated them incorrectly. Head anchors include the body's ground shift.
3. Adds per-frame X/Y head offsets measured from the rider's actual walking
   neck displacement, including mirrored directions. The original conversion
   used constant walking anchors even though the torso shifted between frames.
   Poring squash/bounce is preserved; frames are not individually re-grounded.

`tools/fix_poring_mount_anchors.py` writes separate corrected copies, recognizes
both the original conversion and the previous head-only repair, rejects unknown
profiles, and is idempotent. It changes only body-layer Y and attachment X/Y
fields. All SPRs, palette data, ACT frame references, mirroring, timing and
other layer properties remain untouched. Four generated-fixture Python tests
cover ground contact, frame offsets, migration, idempotence, mirroring and
unknown-input rejection. No asset data is committed.

Updated CPU-rendered comparisons include the ground origin, actual shadow
asset, normal health-bar offset, and mounted Knight reference. Both sexes,
all eight directions and all four walking frames were inspected. Additional
female hairstyles 1, 5, 10, 15 and 20 were inspected. This comparison establishes
sprite-space alignment, not live terrain/camera projection or final gameplay
correctness. The user must still check the revised files in the running game.
Only idle and walking have been calibrated; other single-frame poses remain
experimental.

The revised custom ACTs are installed locally under their existing names in
`~/Downloads/OldRO/data/sprite/custom/`. Both original conversion files and the
previous head-only repair are backed up privately. SPR hashes were checked
unchanged. No ordinary sprites, GRFs, health-bar logic or shadow logic changed.
The existing test executable works with these revised ACTs; no new binary is
required for this alignment revision.

To reproduce the repair from the original conversion files:

```sh
python3 tools/fix_poring_mount_anchors.py \
  --custom-dir /path/to/original/custom \
  --output-dir /path/to/corrected-act
python3 -m unittest discover -s tools -p test_fix_poring_mount_anchors.py -v
```

Back up the original custom ACTs before copying the corrected files into the
client's `data/sprite/custom/` directory. Do not run the repair against unrelated
ACTs. If a running client has already cached the old ACTs, restart once to load
the corrected asset files; subsequent mount/dismount transitions do not require
a restart or map change.

## Tests and build

Use the Go version in `go.mod`:

```sh
CGO_ENABLED=0 go test -tags nofakecgo ./game ./world ./res ./network
GORO_DATA_DIR=/path/to/OldRO CGO_ENABLED=0 go test -tags nofakecgo ./game -run '^TestCashMountSuppliedAssets$' -v
CGO_ENABLED=0 go vet -tags nofakecgo ./...
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags nofakecgo -trimpath -ldflags='-s -w' -o goro-poring-macos-arm64 .
```

Synthetic tests cover raw 2008 status packets, login state, repeated local and
remote transitions, snapshot retention, actor removal, cache separation,
palette restoration, missing/invalid assets, head/headgear attachment transforms,
and all directions. No copyrighted fixture assets are committed.

The existing **Release** workflow now builds a macOS ARM64 artifact on pushes
to `feat/poring-cash-mount`, and supports `workflow_dispatch`. Release-triggered
builds retain their existing platform matrix and release uploads. The test
build artifact is named `goro-macos-arm64-cash-mount`. Once the workflow is on
the default branch, it can also be run from Actions → Release → Run workflow,
selecting the feature branch. Download the artifact from the completed run and
extract `goro-osx-arm64`; downloaded ZIPs may require `chmod +x goro-osx-arm64`.

A local build and `go vet ./...` passed with CGO disabled and `nofakecgo`.
The full macOS test run encountered one existing failure:
`TestGuildEmblemUploadUsesSelectedClientFolder` returns duplicate `Emblem` and
`emblem` entries on this case-insensitive filesystem. The same failure was
reproduced against the unmodified base commit. All other tests in the four
related packages passed with that test excluded; the Cash Mount tests passed
without exclusions.

## Local runtime checklist

Launch the downloaded test executable with the existing client directory:

```sh
chmod +x ./goro-osx-arm64
./goro-osx-arm64 --data-dir "$HOME/Downloads/OldRO"
```

Test male and female Novices with `@mount2` in both directions, without a map
change. Check each facing direction while idle and walking, hair colors, all
three headgear slots, clothing dye restoration, and look changes while mounted.
Use a second player to check mounting in view, entering view already mounted,
dismounting, and leaving/reentering view. Check relogging and map transitions.
Perform missing/invalid asset tests in a separate copied client-data directory.

## Changed source files

- `game/actor_mount.go`: local status selection, actor cash-mount transitions, reload.
- `game/status_effects.go`, `game/login.go`: world/login status routing.
- `world/world.go`: independent remote mount state and snapshot retention.
- `game/world.go`, `game/actor.go`, `game/battle.go`: variant-aware sprite keys and lookup.
- `game/sprite_assets.go`, `game/cash_mount_assets.go`: body-only selection, palette, validation, fallback, IMF bypass.
- `game/sprite_render.go`: appearance variant and independent mounted idle animation.
- `game/ui_assets.go`: consistent local preview fallback selection.
- `res/player_resource.go`, `res/player_resource_test.go`: exact sex-specific custom paths and filename checks.
- `game/cash_mount_test.go`: synthetic and opt-in real-asset coverage.
- `tools/fix_poring_mount_anchors.py`, `tools/test_fix_poring_mount_anchors.py`: reproducible private-asset anchor repair and generated-fixture tests.
- `.github/workflows/release.yml`: test build artifact and manual dispatch.
