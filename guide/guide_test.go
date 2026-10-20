package guide

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, root, path, body string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"src/map/mob.cpp": "// fixture server source", "src/custom/defines_pre.hpp": "#define PRERE\n",
		"src/common/mmo.hpp":   "enum e_job { JOB_NOVICE, JOB_SWORDMAN, JOB_KNIGHT=7, JOB_LORD_KNIGHT=4008, };",
		"conf/map_athena.conf": "db_path: db\nimport: conf/import/map_conf.txt\n", "conf/import/map_conf.txt": "// nothing\n",
		"conf/battle_athena.conf": "import: conf/rates.conf\nimport: conf/override.conf\n", "conf/rates.conf": "base_exp_rate: 200\njob_exp_rate: 300\n", "conf/override.conf": "base_exp_rate: 1200\n",
		"db/item_db.yml":             "Header: {Type: ITEM_DB}\nBody:\n- {Id: 501, AegisName: TestItem, Name: Test item, Type: Healing}\n",
		"db/mob_db.yml":              "Header: {Type: MOB_DB}\nBody:\n- {Id: 1001, AegisName: TEST, Name: Test monster, Level: 5, Hp: 70, BaseExp: 2, JobExp: 3, Element: Water, ElementLevel: 2, Drops: [{Item: TestItem, Rate: 100}]}\n- {Id: 1002, AegisName: OTHER, Name: Other monster, Level: 2, Element: Fire}\nFooter:\n Imports:\n - {Path: db/import/mob_db.yml}\n - {Path: db/re/mob_db.yml, Mode: Renewal}\n",
		"db/import/mob_db.yml":       "Header: {Type: MOB_DB}\nBody:\n- {Id: 1001, Hp: 90, Drops: [{Index: 0, Item: TestItem, Rate: 200}, {Item: TestItem, Rate: 1}]}\n",
		"db/job_stats.yml":           "Header: {Type: JOB_STATS}\nBody:\n- Jobs: {Novice: true, Swordman: true, Knight: true, Lord_Knight: true}\n  MaxBaseLevel: 99\n- Jobs: {Novice: true}\n  MaxJobLevel: 10\n  JobExp: [{Level: 1, Exp: 10}]\n- Jobs: {Swordman: true}\n  MaxJobLevel: 50\n  JobExp: [{Level: 1, Exp: 30}]\n- Jobs: {Knight: true}\n  MaxJobLevel: 50\n  JobExp: [{Level: 1, Exp: 60}]\n- Jobs: {Lord_Knight: true}\n  MaxJobLevel: 70\n  JobExp: [{Level: 1, Exp: 90}]\n",
		"db/mob_item_ratio.yml":      "Header: {Type: MOB_ITEM_RATIO_DB}\nBody: [{Item: TestItem, Ratio: 400, List: {TEST: true}}]\n",
		"db/pre-re/mob_skill_db.txt": "1001,Test,attack,12,2,500,0,1000,yes,target,always,0,,,,,,\n", "npc/pre-re/scripts_main.conf": "npc: npc/test.txt\nnpc: npc/disabled.txt\ndelnpc: npc/disabled.txt\n// npc: npc/missing.txt\n",
		"npc/test.txt": "field,0,0\tmonster\tTest\t1001,5,1000,500\nfield,0,0\tmonster\tOther\t1002,2\nfield,0,0\tmonster\tTest\t1001,3,2000,1000\nfield,20,30,4\tscript\tOutdoor#one\t1,{\n mes \"// not a comment\";\n}\nfield,40,50,0\twarp\tDoor\t1,1,inside,5,5\nfield,60,70,0\twarp\tDoor2\t1,1,inside,6,6\ninside\tmapflag\tindoors\ninside\tmapflag\tnojobexp\ninside,2,3,4\tshop\tVendor\t1,501:10\ninside,5,6,0\twarp\tDeeper\t1,1,basement,1,1\nbasement\tmapflag\tindoors\nbasement,2,2,4\tscript\tQuest\t1,{ end; }\n/*\nfield,0,0\tmonster\tDisabled\t1001,99\n*/\n",
	}
	defaults := ""
	for k, v := range map[string]int{"base_exp_rate": 100, "job_exp_rate": 100, "mob_count_rate": 100, "item_rate_heal": 800, "item_rate_heal_boss": 400, "item_rate_heal_mvp": 200, "item_rate_mvp": 600, "item_rate_treasure": 500, "item_drop_heal_min": 1, "item_drop_heal_max": 10000, "item_drop_mvp_min": 1, "item_drop_mvp_max": 10000, "item_drop_treasure_min": 1, "item_drop_treasure_max": 10000, "item_logarithmic_drops": 0, "max_exp_gain_rate": 0} {
		defaults += fmt.Sprintf("{ \"%s\", &battle_config.%s, %d, 0, 10000 },\n", k, k, v)
	}
	files["src/map/battle.cpp"] = defaults
	for p, b := range files {
		write(t, root, p, b)
	}
	return root
}
func loadFixture(t *testing.T) *Snapshot {
	t.Helper()
	s, e := Load(fixture(t), "auto")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestOrderedMetadataOverridesAndDisabledScripts(t *testing.T) {
	s := loadFixture(t)
	m := s.Monsters[1001]
	if s.Mode != "pre-re" || m.Hp != 90 || m.Level != 5 || m.Element != "Water" || m.ElementLevel != 2 || len(m.Drops) != 2 || m.Drops[0].Rate != 200 {
		t.Fatalf("bad merged monster: %+v", m)
	}
	if len(s.Spawns) != 3 || len(s.NPCs) != 3 || len(s.Warps) != 3 {
		t.Fatalf("declarations %d %d %d", len(s.Spawns), len(s.NPCs), len(s.Warps))
	}
	if s.Rates["base_exp_rate"] != 1200 || s.Reward(m, false) != 24 || s.Reward(m, true) != 9 {
		t.Fatal("ordered rates")
	}
	if s.Changed() {
		t.Fatal("unchanged snapshot dirty")
	}
	write(t, s.Root, "conf/override.conf", "base_exp_rate: 400\n")
	if !s.Changed() {
		t.Fatal("change missed")
	}
	next, e := Load(s.Root, "auto")
	if e != nil || next.Rates["base_exp_rate"] != 400 {
		t.Fatalf("reload %v", e)
	}
}
func TestMapMonstersAndSpawns(t *testing.T) {
	s := loadFixture(t)
	list := s.MapMonsters("field.gat", "")
	if len(list) != 2 || list[0].ID != 1002 {
		t.Fatal(list)
	}
	a := s.SpawnInfo("field", 1001)
	if len(a) != 2 || a[0].Count+a[1].Count != 8 || a[0].Delay == a[1].Delay {
		t.Fatal(a)
	}
	if len(s.MapMonsters("empty", "")) != 0 || len(s.MapMonsters("field", "test")) != 1 {
		t.Fatal("filter")
	}
}
func TestCharacterProgressionAndTransitions(t *testing.T) {
	s := loadFixture(t)
	m := s.Monsters[1001]
	for _, tc := range []struct {
		id   int
		next int64
		want string
	}{{1, 30, "4 kills (estimate)"}, {7, 60, "7 kills (estimate)"}, {4008, 90, "10 kills (estimate)"}} {
		j := s.Jobs[tc.id]
		if j.JobExp[1] != tc.next {
			t.Fatalf("job %d progression %+v", tc.id, j)
		}
		p := Progress{JobID: tc.id, JobLevel: 1, NextJob: tc.next}
		if got := s.Kills(m, p, true); got != tc.want {
			t.Fatalf("job %d: %s", tc.id, got)
		}
	}
	p := Progress{JobID: 1, BaseLevel: 10, JobLevel: 1, NextBase: 100, NextJob: 30}
	if s.Kills(m, p, false) != "5 kills (estimate)" {
		t.Fatal("base")
	}
	p.BaseExp = 24
	if s.Kills(m, p, false) != "4 kills (estimate)" {
		t.Fatal("combat refresh")
	}
	p.JobID = 7
	p.JobLevel = 1
	p.JobExp = 0
	p.NextJob = 60
	if s.Kills(m, p, true) != "7 kills (estimate)" {
		t.Fatal("job reset")
	}
	p.BaseExp = 100
	if s.Kills(m, p, false) != "0 kills (estimate)" {
		t.Fatal("zero remaining")
	}
	p.BaseLevel = 99
	if s.Kills(m, p, false) != "Max level reached" {
		t.Fatal("maxbase")
	}
	p.JobID = 0
	p.JobLevel = 10
	if s.Kills(m, p, true) != "Max level reached" {
		t.Fatal("maxjob")
	}
	p.JobLevel = 1
	p.NextJob = 0
	if s.Kills(m, p, true) != "EXP requirement unavailable" {
		t.Fatal("unknown threshold")
	}
	p.NextJob = 30
	m.JobExp = 0
	if s.Kills(m, p, true) != "No EXP" {
		t.Fatal("zero reward")
	}
}
func TestEXPCapAndMapFlags(t *testing.T) {
	s := loadFixture(t)
	m := s.Monsters[1001]
	s.Rates["max_exp_gain_rate"] = 100
	p := Progress{NextBase: 100, BaseLevel: 1}
	if s.Kills(m, p, false) != "10 kills (estimate)" {
		t.Fatal("cap")
	}
	p.Map = "inside"
	if s.Kills(m, p, true) != "EXP requirement unavailable" {
		t.Fatal("threshold")
	}
	p.NextJob = 30
	if s.Kills(m, p, true) != "No EXP" {
		t.Fatal("nojobexp")
	}
}
func TestDropCategoriesBossOverridesCapsAndLogarithmic(t *testing.T) {
	s := loadFixture(t)
	m := s.Monsters[1001]
	d := Drop{Item: "TestItem", Rate: 200}
	rate, _ := s.DropRate(m, d, false)
	if rate != 800 {
		t.Fatal(rate)
	}
	s.Ratios = nil
	rate, _ = s.DropRate(m, d, false)
	if rate != 1600 {
		t.Fatal(rate)
	}
	m.Class = "Boss"
	rate, _ = s.DropRate(m, d, false)
	if rate != 800 {
		t.Fatal(rate)
	}
	m.Modes = map[string]bool{"Mvp": true}
	rate, _ = s.DropRate(m, d, false)
	if rate != 400 {
		t.Fatal(rate)
	}
	rate, _ = s.DropRate(m, d, true)
	if rate != 1200 {
		t.Fatal(rate)
	}
	s.Rates["item_drop_heal_max"] = 250
	rate, _ = s.DropRate(m, d, false)
	if rate != 250 {
		t.Fatal(rate)
	}
	s.Rates["item_rate_heal_mvp"] = 0
	rate, _ = s.DropRate(m, d, false)
	if rate != 0 {
		t.Fatal("zero disabled rate clamped up")
	}
	m.Modes = nil
	m.Class = ""
	s.Rates["item_drop_heal_max"] = 10000
	s.Rates["item_logarithmic_drops"] = 1
	rate, _ = s.DropRate(m, d, false)
	if rate != 721 {
		t.Fatalf("log drop %d", rate)
	}
	d.Rate = 0
	rate, _ = s.DropRate(m, d, false)
	if rate != 0 {
		t.Fatal("zero raw")
	}
}
func TestVerifiedInteriorEntrances(t *testing.T) {
	s := loadFixture(t)
	out := s.MapNPCs("field", "", nil)
	if len(out) != 3 {
		t.Fatal(out)
	}
	for _, n := range out {
		if n.Interior && len(n.Entrances) != 2 {
			t.Fatal(n)
		}
		if !n.Interior && (len(n.Entrances) != 1 || n.Entrances[0] != (Point{20, 30})) {
			t.Fatal(n)
		}
	}
	in := s.MapNPCs("inside", "", nil)
	if len(in) != 1 || len(in[0].Entrances) != 0 {
		t.Fatal(in)
	}
	if len(s.MapNPCs("field", "vendor", nil)) != 1 || len(s.MapNPCs("other", "", nil)) != 0 {
		t.Fatal("filter")
	}
	s.Warps = append(s.Warps, Warp{Map: "inside", Destination: "farout", X: 1, Y: 1})
	s.NPCs = append(s.NPCs, NPC{Map: "farout", Name: "Unrelated"})
	if len(s.MapNPCs("field", "", nil)) != 3 {
		t.Fatal("traversed another outdoor map")
	}
}
func TestCyclesAndInvalidMetadata(t *testing.T) {
	root := fixture(t)
	write(t, root, "conf/import/map_conf.txt", "import: conf/map_athena.conf\n")
	if _, err := Load(root, "auto"); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatal(err)
	}
	root = fixture(t)
	write(t, root, "db/import/mob_db.yml", "not: [valid")
	if _, err := Load(root, "auto"); err == nil {
		t.Fatal("malformed metadata accepted")
	}
}
func TestAsyncServiceReloadAndMissingFiles(t *testing.T) {
	root := fixture(t)
	var s Service
	s.Refresh(root, "auto")
	deadline := time.Now().Add(3 * time.Second)
	for s.Loading && time.Now().Before(deadline) {
		s.Poll(root, "auto")
		time.Sleep(time.Millisecond)
	}
	if s.Snapshot == nil || s.Error != "" {
		t.Fatal(s.Error)
	}
	old := s.Snapshot
	write(t, root, "conf/override.conf", "base_exp_rate: 400\n")
	s.Refresh(root, "auto")
	for s.Loading && time.Now().Before(deadline) {
		s.Poll(root, "auto")
		time.Sleep(time.Millisecond)
	}
	if s.Snapshot == old || s.Snapshot.Rates["base_exp_rate"] != 400 {
		t.Fatal("reload")
	}
	if err := os.Remove(filepath.Join(root, "npc/test.txt")); err != nil {
		t.Fatal(err)
	}
	s.Refresh(root, "auto")
	for s.Loading && time.Now().Before(deadline) {
		s.Poll(root, "auto")
		time.Sleep(time.Millisecond)
	}
	if s.Snapshot != nil || s.Error == "" {
		t.Fatal("stale metadata shown after failure")
	}
}

func TestRespawnScalingAndMinimum(t *testing.T) {
	s := loadFixture(t)
	sp := Spawn{MobID: 1001, Delay: 10000, Variance: 2000}
	s.Rates["mob_spawn_delay"] = 200
	lo, hi := s.RespawnRange(sp)
	if lo != 20000 || hi != 23800 {
		t.Fatalf("delay %d–%d", lo, hi)
	}
	sp.Delay = 0
	sp.Variance = 0
	lo, hi = s.RespawnRange(sp)
	if lo != 1000 || hi != 1000 {
		t.Fatal("respawn floor")
	}
}
func TestDropMinimumCardAndZeroMapRate(t *testing.T) {
	s := loadFixture(t)
	m := s.Monsters[1001]
	s.Ratios = nil
	i := s.Items["TestItem"]
	i.Type = "Card"
	s.Items["TestItem"] = i
	s.Rates["item_rate_card"] = 1
	s.Rates["item_drop_card_min"] = 5
	s.Rates["item_drop_card_max"] = 10000
	n, _ := s.DropRate(m, Drop{Item: "TestItem", Rate: 1}, false)
	if n != 5 {
		t.Fatal(n)
	}
	s.MapRules["zero"] = MapRules{BaseSet: true, BaseRate: 0}
	if s.MapReward(m, "zero", false) != 1 {
		t.Fatal("rAthena minimum reward floor")
	}
}
