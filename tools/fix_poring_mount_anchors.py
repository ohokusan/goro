#!/usr/bin/env python3
"""Repair head attachments and ground pivots in the experimental Novice ACTs.

Writes separate copies; never changes SPRs. Accepts the original conversion or
the first head-only repair, and refuses unknown input profiles. The calibration
is specific to these converted 136x108, 104-action assets.
"""
import argparse
from dataclasses import dataclass
from pathlib import Path
import struct

ORIGINAL = {
    "male": [(0, -78), (11, -83), (18, -85), (10, -84),
             (0, -83), (-10, -84), (-18, -85), (-11, -83)],
    "female": [(0, -77), (11, -82), (18, -84), (10, -83),
               (0, -82), (-10, -83), (-18, -84), (-11, -82)],
}
# Previous head-only repair, accepted for migration without applying it twice.
HEAD_ONLY = {
    "male": [(2, -65), (6, -66), (5, -65), (28, -84),
             (11, -83), (-27, -84), (-5, -65), (-5, -66)],
    "female": [(0, -66), (3, -69), (5, -67), (28, -85),
               (11, -83), (-27, -85), (-5, -67), (-2, -69)],
}
# Native attachment coordinates derived by registering the rider's neck/collar
# pixels to the ordinary male/female Novice body ACT reference. In particular,
# rear head ACT anchors are above/right of the head center, not at its neck.
# Rounding to integer ACT pixels introduces at most half a source pixel.
BODY_ALIGNED = {
    "male": [(0, -70), (4, -71), (0, -71), (19, -100),
             (12, -102), (-19, -100), (1, -71), (-3, -71)],
    "female": [(0, -68), (6, -69), (3, -70), (18, -98),
               (12, -101), (-18, -98), (-3, -70), (-5, -69)],
}
# The principal Poring body's idle contact row in each 136x108 frame. Decorative
# trailing mini-Porings are excluded: their lowest pixel is not a foot point.
# Put that row at Y=-2 relative to the ground anchor, matching the male Knight's
# rear-facing foot point (~-1.5). Keep health-bar/shadow/world coordinates intact.
CONTACT_ROW = [104, 104, 104, 88, 82, 88, 104, 104]
GROUND_SHIFT = [106 - row for row in CONTACT_ROW]
CORRECTED = {sex: [(x, y + GROUND_SHIFT[d]) for d, (x, y) in enumerate(points)]
             for sex, points in BODY_ALIGNED.items()}

# Neck displacement measured from the actual SPR frames, relative to idle.
# The conversion's walking anchors were constant even when the torso moved.
# These offsets preserve the Poring's squash/bounce instead of re-grounding
# every frame. The head now follows the rider, not the normal walking head bob.
WALK_NECK = {
    "male": [
        [(0, 0), (1, -3), (0, 0), (0, 0)],
        [(0, 0), (0, 4), (0, 1), (1, 2)],
        [(-1, 0), (-4, 4), (-1, -2), (1, 1)],
        [(-4, 4), (-5, 0), (-3, 3), (-3, 7)],
        [(0, -3), (-1, -2), (0, -5), (0, 0)],
    ],
    "female": [
        [(0, 0), (1, -3), (0, 0), (0, 0)],
        [(-1, 0), (0, 4), (0, 1), (1, 2)],
        [(-1, 0), (-4, 4), (-1, -2), (1, 1)],
        [(-4, 4), (-5, 0), (-3, 3), (-3, 7)],
        [(0, -3), (-1, -2), (0, -5), (0, 0)],
    ],
}


def walk_delta(sex, direction, motion):
    source = direction if direction <= 4 else 8 - direction
    x, y = WALK_NECK[sex][source][motion]
    return (-x if direction > 4 else x), y


@dataclass(frozen=True)
class Frame:
    action: int
    motion: int
    layer: int
    anchor: int


def frame_records(data):
    if data[:4] != b"AC\x03\x02" or len(data) < 16:
        raise ValueError("expected ACT version 2.3")
    actions = struct.unpack_from("<H", data, 4)[0]
    if actions != 104:
        raise ValueError("expected the supplied 104-action conversion")
    offset = 16
    records = []

    def read_count():
        nonlocal offset
        if offset + 4 > len(data):
            raise ValueError("truncated ACT count")
        value = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        if value > len(data):
            raise ValueError("invalid ACT count")
        return value

    for action in range(actions):
        frames = read_count()
        if action < 16 and frames != (1 if action < 8 else 4):
            raise ValueError("unexpected idle/walk frame count")
        for motion in range(frames):
            offset += 32
            if read_count() != 1:
                raise ValueError("expected exactly one body layer")
            layer = offset
            offset += 32 + 4  # ACT 2.3 layer and sound index.
            if read_count() != 1:
                raise ValueError("expected exactly one head attachment")
            if offset + 16 > len(data):
                raise ValueError("truncated ACT attachment")
            records.append(Frame(action, motion, layer, offset + 4))
            offset += 16
    return records


def repair(data, sex):
    records = frame_records(data)
    baseline = [None] * 8
    layer_y = [None] * 8
    for f in records:
        if f.action < 8:
            baseline[f.action] = struct.unpack_from("<ii", data, f.anchor)
            layer_y[f.action] = struct.unpack_from("<i", data, f.layer + 4)[0]
    if baseline == CORRECTED[sex] and layer_y == [-54 + dy for dy in GROUND_SHIFT]:
        return data
    if layer_y != [-54] * 8:
        raise ValueError("unknown body origin profile; refusing to alter this ACT")
    if baseline == ORIGINAL[sex]:
        source = ORIGINAL[sex]
    elif baseline == HEAD_ONLY[sex]:
        source = HEAD_ONLY[sex]
    else:
        raise ValueError("unknown anchor profile; refusing to alter this ACT")
    result = bytearray(data)
    for f in records:
        direction = f.action % 8
        x, y = struct.unpack_from("<ii", data, f.anchor)
        old_x, old_y = source[direction]
        new_x, new_y = CORRECTED[sex][direction]
        dx, dy = walk_delta(sex, direction, f.motion) if 8 <= f.action < 16 else (0, 0)
        struct.pack_into("<ii", result, f.anchor,
                         x + new_x - old_x + dx, y + new_y - old_y + dy)
        body_y = struct.unpack_from("<i", data, f.layer + 4)[0]
        struct.pack_into("<i", result, f.layer + 4, body_y + GROUND_SHIFT[direction])
    return bytes(result)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--custom-dir", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    if args.custom_dir.resolve() == args.output_dir.resolve():
        parser.error("use a separate output directory to preserve originals")
    corrected = {}
    for sex in ORIGINAL:
        name = f"poring_novice_{sex}.act"
        corrected[name] = repair((args.custom_dir / name).read_bytes(), sex)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    for name, data in corrected.items():
        path = args.output_dir / name
        path.write_bytes(data)
        print(path)


if __name__ == "__main__":
    main()
