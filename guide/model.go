// Package guide reads configured rAthena metadata without connecting to its databases.
package guide

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type Drop struct {
	Item  string
	Rate  int
	Index *int
}
type Monster struct {
	ID                                                int `yaml:"Id"`
	AegisName, Name                                   string
	Level, Hp, Attack, Attack2, Defense, MagicDefense int
	BaseExp, JobExp                                   int64
	Size, Race, Element, Class                        string
	ElementLevel                                      int
	Modes, RaceGroups                                 map[string]bool
	Drops, MvpDrops                                   []Drop
	Skills                                            []string `yaml:"-"`
}
type Item struct {
	ID                    int `yaml:"Id"`
	AegisName, Name, Type string
}
type Spawn struct {
	Map                                                      string
	MobID, Count, Delay, Variance, X, Y, AreaX, AreaY, Level int
	Source                                                   string
}
type NPC struct {
	ID, Name, Role, Map, Source string
	X, Y                        int
}
type Warp struct {
	Map, Destination string
	X, Y             int
}
type Point struct{ X, Y int }
type LocatedNPC struct {
	NPC
	Entrances []Point
	Interior  bool
}
type Job struct {
	MaxBase, MaxJob int
	BaseExp, JobExp map[int]int64
}
type Ratio struct {
	Item  string
	Ratio int
	List  map[string]bool
}
type MapRules struct {
	BaseRate, JobRate int
	BaseSet, JobSet   bool
	NoBase, NoJob     bool
}
type Snapshot struct {
	MapRules   map[string]MapRules
	Root, Mode string
	Loaded     time.Time
	Monsters   map[int]Monster
	Items      map[string]Item
	Spawns     []Spawn
	NPCs       []NPC
	Warps      []Warp
	Indoor     map[string]bool
	Rates      map[string]int
	Jobs       map[int]Job
	Ratios     []Ratio
	Warnings   []string
	Files      map[string]Stamp
}
type Stamp struct {
	Size     int64
	Modified int64
}

func MapName(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(s, ".gat"), ".rsw"))
}
func (s *Snapshot) MapMonsters(name, search string) []Monster {
	ids := map[int]bool{}
	for _, sp := range s.Spawns {
		if sp.Map == MapName(name) {
			ids[sp.MobID] = true
		}
	}
	out := []Monster{}
	for id := range ids {
		m, ok := s.Monsters[id]
		if ok && strings.Contains(strings.ToLower(m.Name), strings.ToLower(search)) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func (s *Snapshot) SpawnInfo(name string, id int) []Spawn {
	var a []Spawn
	for _, v := range s.Spawns {
		if v.Map == MapName(name) && v.MobID == id {
			a = append(a, v)
		}
	}
	return a
}
func (s *Snapshot) MapNPCs(name, search string, indoor func(string) bool) []LocatedNPC {
	name = MapName(name)
	inside := func(n string) bool { return s.Indoor[n] || (indoor != nil && indoor(n)) }
	routes := map[string][]Point{}
	if !inside(name) {
		// Carry the first outdoor entrance through interior-only paths. Never walk
		// across another outdoor map and invent a building association.
		for _, w := range s.Warps {
			if w.Map != name || !inside(w.Destination) {
				continue
			}
			p := Point{w.X, w.Y}
			seen := map[string]bool{}
			queue := []string{w.Destination}
			for len(queue) > 0 {
				n := queue[0]
				queue = queue[1:]
				if seen[n] {
					continue
				}
				seen[n] = true
				routes[n] = appendUnique(routes[n], p)
				for _, edge := range s.Warps {
					if edge.Map == n && inside(edge.Destination) && !seen[edge.Destination] {
						queue = append(queue, edge.Destination)
					}
				}
			}
		}
	}
	out := []LocatedNPC{}
	for _, n := range s.NPCs {
		if n.Map != name && len(routes[n.Map]) == 0 {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Name), strings.ToLower(search)) {
			continue
		}
		v := LocatedNPC{NPC: n, Interior: n.Map != name, Entrances: routes[n.Map]}
		if n.Map == name && !inside(name) {
			v.Entrances = []Point{{n.X, n.Y}}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func appendUnique(a []Point, p Point) []Point {
	for _, v := range a {
		if v == p {
			return a
		}
	}
	return append(a, p)
}

// Reward is the configured solo reward before character/party runtime effects.
func (s *Snapshot) Reward(m Monster, job bool) int64 {
	raw, key := m.BaseExp, "base_exp_rate"
	if job {
		raw, key = m.JobExp, "job_exp_rate"
	}
	return int64(float64(raw) * float64(s.Rates[key]) / 100)
}

type Progress struct {
	Map                                string
	JobID, BaseLevel, JobLevel         int
	BaseExp, JobExp, NextBase, NextJob int64
}

func (s *Snapshot) Kills(m Monster, p Progress, job bool) string {
	current, next, level, limit := p.BaseExp, p.NextBase, p.BaseLevel, s.Jobs[p.JobID].MaxBase
	if job {
		current, next, level, limit = p.JobExp, p.NextJob, p.JobLevel, s.Jobs[p.JobID].MaxJob
	}
	if limit > 0 && level >= limit {
		return "Max level reached"
	}
	if next <= 0 {
		return "EXP requirement unavailable"
	}
	reward := s.MapReward(m, p.Map, job)
	if cap := s.Rates["max_exp_gain_rate"]; cap > 0 {
		reward = min(reward, int64(float64(next)*float64(cap)/1000))
	}
	if reward <= 0 {
		return "No EXP"
	}
	remaining := max(int64(0), next-current)
	return fmt.Sprintf("%d kills (estimate)", remaining/reward+boolInt(remaining%reward != 0))
}

// MapReward applies static map EXP flags after global database multipliers.
func (s *Snapshot) MapReward(m Monster, name string, job bool) int64 {
	reward := s.Reward(m, job)
	r := s.MapRules[MapName(name)]
	rate := r.BaseRate
	known := r.BaseSet
	blocked := r.NoBase
	if job {
		rate = r.JobRate
		known = r.JobSet
		blocked = r.NoJob
	}
	if blocked {
		return 0
	}
	if !known {
		rate = 100
	}
	if reward == 0 {
		return 0
	}
	return max(1, int64(float64(reward)*float64(rate)/100))
}
func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
func (s *Snapshot) DropRate(m Monster, d Drop, mvp bool) (int, string) {
	item := s.Items[d.Item]
	cat := "common"
	switch item.Type {
	case "Healing":
		cat = "heal"
	case "Usable", "Cash":
		cat = "use"
	case "Weapon", "Armor", "Petarmor", "PetArmor":
		cat = "equip"
	case "Card":
		cat = "card"
	}
	key := "item_rate_" + cat
	capcat := cat
	if mvp {
		key = "item_rate_mvp"
		capcat = "mvp"
	} else if m.RaceGroups["Treasure"] {
		key = "item_rate_treasure"
		capcat = "treasure"
	} else if m.Modes["Mvp"] || m.Modes["MVP"] {
		key += "_mvp"
	} else if m.Class == "Boss" {
		key += "_boss"
	}
	multiplier, known := s.Rates[key]
	if !known {
		return 0, "Rate unavailable"
	}
	for _, r := range s.Ratios {
		if r.Item == d.Item && (len(r.List) == 0 || r.List[m.AegisName]) {
			multiplier = r.Ratio
		}
	}
	raw := d.Rate
	if raw <= 0 || multiplier <= 0 {
		return 0, key
	}
	if !mvp && s.Rates["drop_rateincrease"] != 0 && raw < 5000 {
		raw++
	}
	rate := float64(raw) * float64(multiplier) / 100
	if s.Rates["item_logarithmic_drops"] != 0 && multiplier != 100 {
		rate = float64(raw)*math.Pow(5-math.Log10(float64(raw)), math.Log(float64(multiplier)/100)/math.Log(5)) + .5
	}
	lo, lok := s.Rates["item_drop_"+capcat+"_min"]
	hi, hok := s.Rates["item_drop_"+capcat+"_max"]
	if !lok || !hok {
		return 0, "Caps unavailable"
	}
	return min(10000, max(lo, min(hi, int(rate)))), fmt.Sprintf("%s ×%.2g", key, float64(multiplier)/100)
}
func (s *Snapshot) RespawnRange(sp Spawn) (int, int) {
	m := s.Monsters[sp.MobID]
	key := "mob_spawn_delay"
	if m.Modes["StatusImmune"] || m.Class == "Boss" || m.Class == "Guardian" || m.Class == "Battlefield" {
		key = "boss_spawn_delay"
	} else if m.Modes["IgnoreMelee"] || m.Modes["IgnoreMagic"] || m.Modes["IgnoreRanged"] || m.Modes["IgnoreMisc"] {
		key = "plant_spawn_delay"
	}
	rate, ok := s.Rates[key]
	if !ok {
		rate = 100
	}
	scale := func(n int) int {
		if rate != 100 {
			n = n / 100 * rate
		}
		return max(1000, n)
	}
	return scale(sp.Delay), scale(sp.Delay + max(0, sp.Variance-1))
}
func SpawnText(a []Spawn) string {
	if len(a) == 0 {
		return "Configured spawn unavailable"
	}
	n := 0
	parts := []string{}
	for _, v := range a {
		n += v.Count
		timing := "minimum 1s respawn"
		if v.Delay+v.Variance > 0 {
			timing = fmt.Sprintf("%s–%s raw delay (before spawn-delay rates; minimum 1s)", time.Duration(v.Delay)*time.Millisecond, time.Duration(v.Delay+v.Variance)*time.Millisecond)
		}
		parts = append(parts, fmt.Sprintf("%d: %s", v.Count, timing))
	}
	return fmt.Sprintf("%d configured · %s", n, strings.Join(parts, "; "))
}
