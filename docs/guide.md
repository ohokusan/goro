# Guide

Open **Guide** beside the Basic Menu collapse control. The original eight menu
buttons retain their 4×2 layout. Guide is a movable, closable native window with
horizontal Monsters / NPCs tabs, search, independent list/detail scrolling and
per-tab selection. Its dimensions adapt to the game window. Search focus blocks
character shortcuts; the rest of the game remains playable outside the window.

## Configure the source

The default is `~/rathena`. Add this to the client data folder's `goro.ini`:

```ini
[guide]
server_root = /Users/yourname/rathena
mode = pre-re
```

Or launch with `--guide-server-root /path/to/rathena --guide-mode pre-re`.
`mode` accepts `auto`, `pre-re`, `re`. Auto recognizes an active `#define PRERE`
in `src/custom/defines_pre.hpp`; otherwise it chooses renewal. Set the mode
explicitly if the server is compiled using other build flags. Protocol date
alone does not determine renewal rules. The client cannot inspect which settings
a running server process has loaded.

**Refresh** forces a metadata read. While Guide is open, a worker checks the
previously read files' modification times and sizes every ten seconds and reloads
changes. Parsing and filesystem checks do not run on the rendering thread.
The footer identifies the configured snapshot and its read time. Configuration
changes only become live server changes when rAthena reloads them independently.
A failed reload clears the snapshot rather than displaying outdated data as live.
No server writes or internet access are required during gameplay.

## Data and calculations

* `conf/map_athena.conf` and ordered imports: only DB paths, SQL mode, NPC entries
  and NPC removal entries are retained. No account/SQL credentials are exported.
* Battle defaults come from the local `src/map/battle.cpp` table. Ordered
  `conf/battle_athena.conf` imports override them, including `conf/import`.
* Root item, monster, job and item-ratio YAML databases follow their Footer
  imports and Renewal/Prerenewal filters. Partial overrides are merged; indexed
  monster drops replace slots, unindexed drops append within rAthena's normal
  10/3 drop limits. Source trees customizing those compile-time limits need a
  matching loader update.
* The mode's `npc/*/scripts_main.conf` and imported configuration determine the
  enabled script files. Commented files and `delnpc` entries are excluded. Only
  top-level static monster/NPC/warp/mapflag declarations are read. Script bodies
  are not executed. Separate spawn intervals, coordinates and level overrides
  remain visible within each monster's combined list entry. Population scaling,
  spawn-delay categories and the server's one-second respawn floor are applied.
* Skill records come from the mode-specific `mob_skill_db.txt`, then import
  overrides, including `clear` and global boss/normal skill groups. Skill IDs,
  levels, states, configured roll chances and delays are shown; chances and
  delays include battle multipliers. They are conditional AI rolls, not a
  promise that a skill will be cast on every attack.
* Global Base/Job reward multipliers and static `bexp`, `jexp`, `nobaseexp`,
  `nojobexp`, `noexp` mapflags affect rewards. Remaining EXP and next-level
  requirements come from the character's server packets, separately for Base
  and Job. They update after combat and job changes. Local job limits and
  progression tables are read using IDs from that server's job enum; no
  universal EXP table is bundled. The per-award `max_exp_gain_rate` cap uses
  rAthena's per-thousand units. No-EXP and maximum-level states avoid division
  by zero. Zero remaining EXP produces zero kills.
* Drop baseline rolls include healing/usable/equipment/card/common categories,
  miniboss/MVP/treasure rates, item-ratio overrides, zero-rate removal,
  logarithmic adjustment, `drop_rateincrease` and category caps. Repeated item
  slots are shown separately. MVP prize rolls are selection-dependent, not
  guaranteed independent per-kill probabilities.
* NPC roles are labelled conservatively (`Shop` for shop declarations, otherwise
  `NPC`). Outdoor minimaps come from existing client resources and GAT map
  dimensions. Red crosses use actual NPC coordinates or the first outdoor warp
  of a verified path through indoor maps. Multiple entrances remain separate.
  Traversal never crosses another outdoor map. Indoor classification uses
  `indoorrswtable.txt` or the server's static `indoors` mapflag, never image
  colour. Interior maps show only their own NPCs and omit the map/coordinates.

## Scope and limitations

This is a configured-data guide, not a live-server monitoring API. The old game
protocol does not transmit server databases, battle rates, spawn timers or NPC
script state. SQL-backed monster/item databases are rejected rather than replaced
with unrelated YAML values. Missing or malformed required files show an error.
Missing optional import DBs are tracked so creating one triggers a reload.

EXP and drop values are labelled **baseline estimates**. Runtime party sharing,
damage contribution, guild tax, VIP, consumables, equipment, script changes,
map-drop scripts, renewal level penalties and temporary player modifiers are not
fully observable through this integration and are not included. Kills assume
solo full-contribution rewards and exclude MVP EXP prizes. Level-specific spawn
overrides are displayed, but runtime stat scaling is not simulated. A running
server with different compiled settings or modified C++ reward logic may differ.

NPC OnInit logic, enable/disable commands, conditional declarations, dynamically
created NPCs/monsters, instances and event activity cannot be evaluated by this
read-only parser. Listings say **configured**, not currently available. Warps
must be literal static declarations; duplicate/generated warps are not expanded.
An indoor NPC with no verified route cannot be associated with an outdoor map
and is therefore listed only on its own interior map. Names/coordinates are
never used to guess a building. Missing artwork yields a text fallback. Preview
sprites, icons and failed resource lookups are cached lazily.

## Build and test

No local Go installation is needed for the downloadable build. In GitHub open
**Actions → Release → Run workflow**, choose `feat/native-guide` (or main after
merge). Download **goro-macos-arm64-preview** from the completed run. It contains
`goro-osx-arm64` and the optional `guide-export-osx-arm64`. Make the executable
runnable with `chmod +x goro-osx-arm64`, and launch it with your existing
`--data-dir` and the settings above. The build uses `CGO_ENABLED=0` and
`-tags=nofakecgo` and includes the merged Cash Mount support.

The optional diagnostic reads the same sources without launching the game:

```sh
./guide-export-osx-arm64 --server-root ~/rathena --mode pre-re > guide-snapshot.json
```

The game reads the server source directly; this JSON is for inspection, not a
second source of settings. It includes source paths and public game metadata.
Do not commit your server snapshot or client sprite assets to the public repo.

Developer checks:

```sh
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo go test ./guide ./ui ./config ./game ./world ./res ./network ./cmd/guide-export
CGO_ENABLED=0 GOFLAGS=-tags=nofakecgo GOOS=darwin GOARCH=arm64 go build .
```

Unit tests cover imports/custom overrides, comments/disabled scripts, reloads,
malformed input, spawn aggregation/sorting/filtering, job-tier thresholds and
resets, caps, maximum/no-EXP states, changing EXP, drop adjustments, interior
warp paths/multiple entrances, missing resources, selection/search/tabs,
published row input and 800×600 / 640×480 / 400×300 layouts. These checks do not
constitute final in-game graphical verification against your server and assets.
