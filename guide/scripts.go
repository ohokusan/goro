package guide

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

// stripComments preserves strings, tabs and newlines so script declarations and
// import paths cannot be accidentally enabled by text inside comments.
func stripComments(s string) string {
	b := []byte(s)
	quoted, escape, block, line := false, false, false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if line {
			if c == '\n' {
				line = false
			} else {
				b[i] = ' '
			}
			continue
		}
		if block {
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				b[i], b[i+1] = ' ', ' '
				i++
				block = false
			} else if c != '\n' {
				b[i] = ' '
			}
			continue
		}
		if quoted {
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == '/' && i+1 < len(b) {
			if b[i+1] == '/' {
				b[i], b[i+1] = ' ', ' '
				i++
				line = true
			} else if b[i+1] == '*' {
				b[i], b[i+1] = ' ', ' '
				i++
				block = true
			}
		}
	}
	return string(b)
}
func (l *loader) removeNPC(path string) {
	if path == "all" {
		l.npcFiles = nil
		return
	}
	a := l.npcFiles[:0]
	for _, f := range l.npcFiles {
		if f != path {
			a = append(a, f)
		}
	}
	l.npcFiles = a
}
func (l *loader) scripts(path string) error {
	return l.config(path, func(k, v string) {
		switch k {
		case "npc":
			for _, f := range l.npcFiles {
				if f == v {
					return
				}
			}
			l.npcFiles = append(l.npcFiles, v)
		case "delnpc":
			l.removeNPC(v)
		}
	})
}
func ints(s string) []int {
	a := strings.Split(s, ",")
	out := make([]int, len(a))
	for i, v := range a {
		out[i], _ = strconv.Atoi(strings.TrimSpace(v))
	}
	return out
}
func (l *loader) scriptFile(path string) error {
	b, err := l.read(path)
	if err != nil {
		return fmt.Errorf("guide cannot read enabled NPC file %s", path)
	}
	depth := 0
	quoted, escape := false, false
	for lineNo, line := range strings.Split(stripComments(string(b)), "\n") {
		if depth == 0 {
			fields := strings.Split(strings.TrimSpace(line), "\t")
			clean := fields[:0]
			for _, v := range fields {
				if strings.TrimSpace(v) != "" {
					clean = append(clean, strings.TrimSpace(v))
				}
			}
			fields = clean
			if len(fields) >= 3 {
				pos := strings.Split(fields[0], ",")
				name := MapName(pos[0])
				kind := fields[1]
				switch {
				case kind == "mapflag":
					on := true
					if len(fields) > 3 && fields[3] == "off" {
						on = false
					}
					if fields[2] == "indoors" {
						l.s.Indoor[name] = on
					}
					r := l.s.MapRules[name]
					switch fields[2] {
					case "nobaseexp":
						r.NoBase = on
					case "nojobexp":
						r.NoJob = on
					case "noexp":
						r.NoBase = on
						r.NoJob = on
					case "bexp":
						if len(fields) > 3 {
							r.BaseRate = number(fields[3])
							r.BaseSet = true
							r.NoBase = false
						}
					case "jexp":
						if len(fields) > 3 {
							r.JobRate = number(fields[3])
							r.JobSet = true
							r.NoJob = false
						}
					}
					l.s.MapRules[name] = r
				case kind == "monster" || kind == "boss_monster":
					if len(fields) >= 4 {
						v := strings.Split(fields[3], ",")
						if len(v) < 2 {
							break
						}
						id := number(v[0])
						if id == 0 {
							for key, m := range l.s.Monsters {
								if m.AegisName == v[0] {
									id = key
									break
								}
							}
						}
						if id <= 0 {
							break
						}
						p := ints(fields[0])
						get := func(ix int) int {
							if ix < len(p) {
								return p[ix]
							}
							return 0
						}
						a := ints(fields[3])
						delay, variance := 0, 0
						if len(a) > 2 {
							delay = a[2]
						}
						if len(a) > 3 {
							variance = a[3]
						}
						count := a[1]
						if count > 1 {
							if r, ok := l.s.Rates["mob_count_rate"]; ok {
								count = max(1, count*r/100)
							}
						}
						if count <= 0 {
							break
						}
						level := 0
						mobname := strings.Split(fields[2], ",")
						if len(mobname) > 1 {
							level = number(mobname[1])
						}
						l.s.Spawns = append(l.s.Spawns, Spawn{Map: name, MobID: id, Count: count, Delay: delay, Variance: variance, X: get(1), Y: get(2), AreaX: get(3), AreaY: get(4), Level: level, Source: fmt.Sprintf("%s:%d", path, lineNo+1)})
					}
				case kind == "warp":
					if len(pos) >= 3 && len(fields) >= 4 {
						v := strings.Split(fields[3], ",")
						if len(v) >= 5 {
							l.s.Warps = append(l.s.Warps, Warp{Map: name, X: number(pos[1]), Y: number(pos[2]), Destination: MapName(v[2])})
						}
					}
				case kind == "script" || kind == "shop" || kind == "cashshop" || kind == "itemshop" || kind == "pointshop" || strings.HasPrefix(kind, "duplicate("):
					if len(pos) >= 3 && name != "-" && len(fields) >= 4 {
						sprite := strings.TrimSpace(strings.Split(fields[3], ",")[0])
						if sprite == "-1" {
							break
						}
						display := strings.Split(fields[2], "#")[0]
						role := "NPC"
						if strings.Contains(kind, "shop") {
							role = "Shop"
						}
						l.s.NPCs = append(l.s.NPCs, NPC{ID: fmt.Sprintf("%s:%d", path, lineNo+1), Name: display, Role: role, Map: name, X: number(pos[1]), Y: number(pos[2]), Source: path})
					}
				}
			}
		}
		for _, c := range line {
			if quoted {
				if escape {
					escape = false
				} else if c == '\\' {
					escape = true
				} else if c == '"' {
					quoted = false
				}
				continue
			}
			if c == '"' {
				quoted = true
			} else if c == '{' {
				depth++
			} else if c == '}' {
				depth = max(0, depth-1)
			}
		}
	}
	return nil
}
func (l *loader) skills() {
	rows := map[int][]string{}
	for _, path := range []string{filepath.Join(l.db, l.s.Mode, "mob_skill_db.txt"), filepath.Join(l.db, "import/mob_skill_db.txt")} {
		b, err := l.read(path)
		if err != nil {
			if strings.Contains(path, "/import/") {
				continue
			}
			l.s.Warnings = append(l.s.Warnings, "Monster skill database unavailable")
			continue
		}
		r := csv.NewReader(strings.NewReader(stripComments(string(b))))
		r.FieldsPerRecord = -1
		r.TrimLeadingSpace = true
		r.LazyQuotes = true
		for {
			v, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				l.s.Warnings = append(l.s.Warnings, "Some monster skill records could not be parsed")
				break
			}
			if len(v) < 2 {
				continue
			}
			id := number(v[0])
			if v[1] == "clear" {
				delete(rows, id)
				continue
			}
			if len(v) != 19 {
				continue
			}
			rate, delay := number(v[5]), number(v[7])
			if r, ok := l.s.Rates["mob_skill_rate"]; ok {
				if r == 0 {
					continue
				}
				rate = max(1, min(10000, rate*r/100))
			}
			if r, ok := l.s.Rates["mob_skill_delay"]; ok {
				delay = delay * r / 100
			}
			rows[id] = append(rows[id], fmt.Sprintf("Skill %s Lv.%s · %s · %.2f%% roll · %dms delay", v[3], v[4], v[2], float64(rate)/100, delay))
		}
	}
	for id, m := range l.s.Monsters {
		m.Skills = append(m.Skills, rows[id]...)
		immune := m.Modes["StatusImmune"] || m.Class == "Boss" || m.Class == "Guardian" || m.Class == "Battlefield"
		for _, g := range []int{-1, -2, -3} {
			mask := -g
			if (immune && mask&1 != 0) || (!immune && mask&2 != 0) {
				m.Skills = append(m.Skills, rows[g]...)
			}
		}
		l.s.Monsters[id] = m
	}
}
