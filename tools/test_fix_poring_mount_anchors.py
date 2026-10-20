"""Generated fixtures only; no Ragnarok assets."""
import struct
import unittest

from fix_poring_mount_anchors import (
    ORIGINAL, HEAD_ONLY, CORRECTED, CONTACT_ROW, GROUND_SHIFT,
    frame_records, repair, walk_delta,
)


def fixture(sex, profile=ORIGINAL):
    data = bytearray(b"AC\x03\x02" + struct.pack("<H", 104) + bytes(10))
    for action in range(104):
        frames = 4 if 8 <= action < 16 else 1
        data += struct.pack("<I", frames)
        for motion in range(frames):
            x, y = profile[sex][action % 8]
            data += bytes(32) + struct.pack("<I", 1)
            data += struct.pack("<iiii", 0, -54, 0, int(action % 8 > 4)) + bytes(16)
            data += struct.pack("<iIiiii", -1, 1, 0, x, y, 0)
    return bytes(data)


class AnchorRepairTest(unittest.TestCase):
    def test_ground_head_walk_and_only_intended_bytes_change(self):
        for sex in ORIGINAL:
            with self.subTest(sex=sex):
                before = fixture(sex)
                after = repair(before, sex)
                self.assertEqual(len(before), len(after))
                allowed = set()
                for f in frame_records(before):
                    d = f.action % 8
                    x, y = CORRECTED[sex][d]
                    dx, dy = walk_delta(sex, d, f.motion) if 8 <= f.action < 16 else (0, 0)
                    self.assertEqual(struct.unpack_from("<ii", after, f.anchor), (x + dx, y + dy))
                    body_y = struct.unpack_from("<i", after, f.layer + 4)[0]
                    self.assertEqual(body_y, -54 + GROUND_SHIFT[d])
                    # Principal body contact, not trailing particles, meets ground.
                    self.assertEqual(body_y - 54 + CONTACT_ROW[d], -2)
                    allowed.update(range(f.anchor, f.anchor + 8))
                    allowed.update(range(f.layer + 4, f.layer + 8))
                self.assertTrue(all(a == b or i in allowed
                                    for i, (a, b) in enumerate(zip(before, after))))
                self.assertEqual(after, repair(after, sex))

    def test_upgrade_first_repair_matches_fresh_original(self):
        for sex in ORIGINAL:
            self.assertEqual(repair(fixture(sex), sex), repair(fixture(sex, HEAD_ONLY), sex))

    def test_mirrored_walk_follows_same_vertical_bob(self):
        for sex in ORIGINAL:
            for d in [1, 2, 3]:
                for motion in range(4):
                    x, y = walk_delta(sex, d, motion)
                    self.assertEqual(walk_delta(sex, 8 - d, motion), (-x, y))

    def test_invalid_and_unknown_profiles_are_rejected(self):
        for data in [b"bad", fixture("male")[:-20], fixture("male")[:40]]:
            with self.assertRaises(ValueError):
                repair(data, "male")
        for field in ["anchor", "origin"]:
            data = bytearray(fixture("male"))
            f = frame_records(data)[0]
            struct.pack_into("<i", data, f.anchor if field == "anchor" else f.layer + 4, 999)
            with self.assertRaises(ValueError):
                repair(data, "male")


if __name__ == "__main__":
    unittest.main()
