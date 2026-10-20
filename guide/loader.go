package guide

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type loader struct {
	s        *Snapshot
	active   map[string]bool
	db       string
	docs     map[string][]map[string]any
	npcFiles []string
	sql      bool
}

// Load follows rAthena imports in order. No SQL connection, process inspection,
// account configuration export, or execution of NPC script code is performed.
func Load(root, mode string) (*Snapshot, error) {
	if strings.HasPrefix(root, "~/") {
		h, _ := os.UserHomeDir()
		root = filepath.Join(h, root[2:])
	}
	root, _ = filepath.Abs(root)
	l := &loader{s: &Snapshot{Root: root, Mode: mode, Loaded: time.Now(), Monsters: map[int]Monster{}, Items: map[string]Item{}, Indoor: map[string]bool{}, MapRules: map[string]MapRules{}, Rates: map[string]int{}, Jobs: map[int]Job{}, Files: map[string]Stamp{}}, active: map[string]bool{}, docs: map[string][]map[string]any{}, db: "db"}
	if _, err := os.Stat(filepath.Join(root, "src/map/mob.cpp")); err != nil {
		return nil, fmt.Errorf("rAthena source not found at %s", root)
	}
	if mode == "" || mode == "auto" {
		defines, _ := l.read("src/custom/defines_pre.hpp")
		if regexp.MustCompile(`(?m)^\s*#\s*define\s+PRERE\b`).MatchString(stripComments(string(defines))) {
			mode = "pre-re"
		} else {
			mode = "re"
		}
		l.s.Mode = mode
	}
	if mode != "pre-re" && mode != "re" {
		return nil, fmt.Errorf("Guide mode must be auto, pre-re or re")
	}
	// Import graph readers retain only whitelisted public metadata.
	if err := l.config("conf/map_athena.conf", func(k, v string) {
		switch k {
		case "use_sql_db":
			l.sql = configInt(v) != 0
		case "db_path":
			l.db = v
		case "npc":
			l.npcFiles = append(l.npcFiles, v)
		case "delnpc":
			l.removeNPC(v)
		}
	}); err != nil {
		return nil, err
	}
	if l.sql {
		return nil, fmt.Errorf("Guide cannot read SQL-backed monster/item databases")
	}
	source, err := l.read("src/map/battle.cpp")
	if err != nil {
		return nil, err
	}
	defaults := regexp.MustCompile(`\{\s*"([a-zA-Z0-9_]+)"\s*,\s*&battle_config\.\w+\s*,\s*(-?\d+)`)
	for _, m := range defaults.FindAllStringSubmatch(string(source), -1) {
		l.s.Rates[m[1]], _ = strconv.Atoi(m[2])
	}
	if err = l.config("conf/battle_athena.conf", func(k, v string) {
		if _, ok := l.s.Rates[k]; ok {
			l.s.Rates[k] = configInt(v)
		}
	}); err != nil {
		return nil, err
	}
	for _, key := range []string{"base_exp_rate", "job_exp_rate"} {
		if _, ok := l.s.Rates[key]; !ok {
			return nil, fmt.Errorf("server battle default %s unavailable", key)
		}
	}
	for _, name := range []string{"item_db.yml", "mob_db.yml", "job_stats.yml", "mob_item_ratio.yml"} {
		if err = l.database(filepath.Join(l.db, name), false); err != nil {
			return nil, err
		}
	}
	if err = l.materialize(); err != nil {
		return nil, err
	}
	// The server loads the mode's default scripts after map configuration.
	if err = l.scripts("npc/" + mode + "/scripts_main.conf"); err != nil {
		return nil, err
	}
	for _, f := range l.npcFiles {
		if err = l.scriptFile(f); err != nil {
			return nil, err
		}
	}
	l.skills()
	l.s.Warnings = append(l.s.Warnings, "Configured snapshot, not live server state. Reload after server configuration changes.", "Runtime NPC enable/disable, scripted spawns, event conditions and instance scripts are not executed.", "EXP estimates assume a solo kill with full contribution. Party sharing, guild tax, VIP, equipment, buffs and renewal level penalties are not included.", "Drop percentages are configured baseline rolls. Player bonuses, renewal level penalties, scripted/map drops and MVP prize selection can change the outcome.")
	return l.s, nil
}
func (l *loader) read(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.s.Root, path)
	}
	st, err := os.Stat(path)
	if err != nil {
		l.s.Files[path] = Stamp{}
		return nil, err
	}
	l.s.Files[path] = Stamp{st.Size(), st.ModTime().UnixNano()}
	return os.ReadFile(path)
}
func configInt(v string) int {
	switch strings.ToLower(v) {
	case "yes", "true", "on":
		return 1
	case "no", "false", "off":
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}
func (l *loader) enter(path string) error {
	if l.active[path] {
		return fmt.Errorf("cyclic metadata import: %s", path)
	}
	l.active[path] = true
	return nil
}
func (l *loader) config(path string, accept func(string, string)) error {
	if err := l.enter(path); err != nil {
		return err
	}
	defer delete(l.active, path)
	b, err := l.read(path)
	if err != nil {
		return fmt.Errorf("Guide cannot read configuration %s", path)
	}
	scan := bufio.NewScanner(strings.NewReader(stripComments(string(b))))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "import" {
			if err = l.config(v, accept); err != nil {
				return err
			}
		} else {
			accept(k, v)
		}
	}
	return scan.Err()
}
func (l *loader) database(path string, optional bool) error {
	if err := l.enter(path); err != nil {
		return err
	}
	defer delete(l.active, path)
	b, err := l.read(path)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("Guide cannot read database %s", path)
	}
	var d struct {
		Header struct {
			Type string `yaml:"Type"`
		} `yaml:"Header"`
		Body   []map[string]any `yaml:"Body"`
		Footer struct {
			Imports []struct {
				Path string `yaml:"Path"`
				Mode string `yaml:"Mode"`
			} `yaml:"Imports"`
		} `yaml:"Footer"`
	}
	if err = yaml.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("invalid Guide database %s: %w", path, err)
	}
	l.docs[d.Header.Type] = append(l.docs[d.Header.Type], d.Body...)
	for _, im := range d.Footer.Imports {
		if im.Mode != "" && ((im.Mode == "Prerenewal") != (l.s.Mode == "pre-re")) {
			continue
		}
		if err = l.database(im.Path, strings.Contains(filepath.ToSlash(im.Path), "/import/")); err != nil {
			return err
		}
	}
	return nil
}
func merge(dst, src map[string]any) {
	for k, v := range src {
		if dm, ok := dst[k].(map[string]any); ok {
			if sm, ok := v.(map[string]any); ok {
				merge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}
func number(v any) int { n, _ := strconv.Atoi(fmt.Sprint(v)); return n }
func decode(m map[string]any, out any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
func (l *loader) materialize() error {
	for _, kind := range []string{"ITEM_DB", "MOB_DB"} {
		all := map[int]map[string]any{}
		for _, row := range l.docs[kind] {
			id := number(row["Id"])
			if all[id] == nil {
				all[id] = map[string]any{}
			}
			if kind == "MOB_DB" {
				for _, key := range []string{"Drops", "MvpDrops"} {
					if incoming, ok := row[key].([]any); ok {
						old, _ := all[id][key].([]any)
						limit := 10
						if key == "MvpDrops" {
							limit = 3
						}
						for _, v := range incoming {
							r, ok := v.(map[string]any)
							if !ok {
								continue
							}
							ix := len(old)
							if n, exists := r["Index"]; exists {
								ix = number(n)
							}
							if ix < 0 || ix > len(old) || ix >= limit {
								continue
							}
							if ix == len(old) {
								old = append(old, r)
							} else {
								old[ix] = r
							}
						}
						all[id][key] = old
						delete(row, key)
					}
				}
			}
			merge(all[id], row)
		}
		for id, row := range all {
			if kind == "ITEM_DB" {
				var i Item
				if err := decode(row, &i); err != nil {
					return err
				}
				l.s.Items[i.AegisName] = i
			} else {
				m := Monster{Level: 1, Hp: 1, ElementLevel: 1, Size: "Small", Race: "Formless", Element: "Neutral"}
				if err := decode(row, &m); err != nil {
					return err
				}
				l.s.Monsters[id] = m
			}
		}
	}
	// Job IDs come from this server's enum, never from a universal EXP table.
	b, err := l.read("src/common/mmo.hpp")
	if err != nil {
		return err
	}
	text := stripComments(string(b))
	start := strings.Index(text, "enum e_job")
	if start < 0 {
		return fmt.Errorf("server job enum unavailable")
	}
	text = text[start:]
	end := strings.Index(text, "};")
	if end < 0 {
		return fmt.Errorf("invalid server job enum")
	}
	text = text[:end]
	ids := map[string]int{}
	n := 0
	re := regexp.MustCompile(`\bJOB_([A-Z0-9_]+)\s*(?:=\s*([0-9]+))?\s*,`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		ids[strings.ReplaceAll(m[1], "_", "")] = n
		n++
	}
	for _, row := range l.docs["JOB_STATS"] {
		jobs, _ := row["Jobs"].(map[string]any)
		for name, on := range jobs {
			if on != true {
				continue
			}
			id, ok := ids[strings.ToUpper(strings.ReplaceAll(name, "_", ""))]
			if !ok {
				continue
			}
			j := l.s.Jobs[id]
			if v, ok := row["MaxBaseLevel"]; ok {
				j.MaxBase = number(v)
			}
			if v, ok := row["MaxJobLevel"]; ok {
				j.MaxJob = number(v)
			}
			for _, k := range []string{"BaseExp", "JobExp"} {
				entries, _ := row[k].([]any)
				dest := j.BaseExp
				if k == "JobExp" {
					dest = j.JobExp
				}
				if dest == nil {
					dest = map[int]int64{}
				}
				for _, v := range entries {
					if e, ok := v.(map[string]any); ok {
						dest[number(e["Level"])] = int64(number(e["Exp"]))
					}
				}
				if k == "BaseExp" {
					j.BaseExp = dest
				} else {
					j.JobExp = dest
				}
			}
			l.s.Jobs[id] = j
		}
	}
	ratios := map[string]Ratio{}
	for _, row := range l.docs["MOB_ITEM_RATIO_DB"] {
		key := fmt.Sprint(row["Item"])
		r := ratios[key]
		r.Item = key
		if v, ok := row["Ratio"]; ok {
			r.Ratio = number(v)
		}
		if r.List == nil {
			r.List = map[string]bool{}
		}
		if m, ok := row["List"].(map[string]any); ok {
			for k, v := range m {
				if v == true {
					r.List[k] = true
				} else {
					delete(r.List, k)
				}
			}
		}
		ratios[key] = r
	}
	for _, r := range ratios {
		l.s.Ratios = append(l.s.Ratios, r)
	}
	return nil
}
