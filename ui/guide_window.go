package ui

import (
	"fmt"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"

	"github.com/gogpu/ui/core/scrollview"
	"github.com/gogpu/ui/core/textfield"
	"github.com/gogpu/ui/event"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/state"
	"github.com/gogpu/ui/widget"
	"github.com/kivutar/goro/guide"
	"github.com/kivutar/goro/res"
	"github.com/kivutar/goro/ui/rotheme"
)

type guideTab struct{ key, label, empty string }

var guideTabs = []guideTab{{"monsters", "Monsters", "No monsters on the map"}, {"npcs", "NPCs", "No NPCs on this map"}}

type GuideWindow struct {
	Window
	service                  guide.Service
	tab                      int
	search                   map[int]string
	selected                 map[int]string
	mapName                  string
	snapshot                 *guide.Snapshot
	progress                 guide.Progress
	list, detail             *guideSlot
	field                    *textfield.Widget
	row                      state.Signal[int]
	statusText               state.Signal[string]
	listScroll, detailScroll state.Signal[float32]
	monsters                 []guide.Monster
	npcs                     []guide.LocatedNPC
	preview                  func(int, int, int) image.Image
	previews                 map[int]image.Image
	icons                    map[int]image.Image
	mapImage                 image.Image
	mapImageName             string
	dirty                    bool
	ctx                      Context
}

func guideSize(sw, sh int) (int, int) { return max(280, min(790, sw-16)), max(210, min(540, sh-16)) }
func (w *GuideWindow) Toggle(ctx Context) {
	if w.IsOpen() {
		w.Close()
		return
	}
	w.ctx = ctx
	sw, sh := ctx.ScreenSize()
	width, height := guideSize(sw, sh)
	w.EnsureWindow(width, height)
	w.SetSize(width, height)
	w.mapName = ""
	w.Window.Open(ctx, w.tree())
	w.dirty = true
	if w.statusText != nil {
		w.statusText.Set(w.status())
	}
	w.Publish(ctx)
}
func (w *GuideWindow) Rebind(ctx Context) {
	if w.IsOpen() {
		w.ctx = ctx
		w.mapName = ""
		w.field = nil
		w.RebindContent(ctx, w.tree())
		w.dirty = true
	}
}
func (w *GuideWindow) KeyboardShortcutsBlocked() bool {
	return w.IsOpen() && w.field != nil && w.field.IsFocused()
}

// Present runs on the game thread. The service parses files in a worker; only
// selected or visible rows request cached graphical resources here.
func (w *GuideWindow) Present(ctx Context, preview func(int, int, int) image.Image) {
	if !w.IsOpen() {
		return
	}
	w.ctx = ctx
	w.preview = preview
	root, mode := ctx.Config.GuideServerRoot, ctx.Config.GuideMode
	if root == "" {
		root = "~/rathena"
	}
	if mode == "" {
		mode = "auto"
	}
	if w.service.Poll(root, mode) {
		w.dirty = true
	}
	name := ""
	if ctx.World != nil {
		name = guide.MapName(ctx.World.MapName)
	}
	p := guide.Progress{}
	if ctx.Session != nil {
		s := ctx.Session
		p = guide.Progress{Map: name, JobID: int(s.SelectedCharacter().Job), BaseLevel: s.Progress.BaseLevel, JobLevel: s.Progress.JobLevel, BaseExp: s.Progress.BaseExp, JobExp: s.Progress.JobExp, NextBase: s.Progress.NextBaseExp, NextJob: s.Progress.NextJobExp}
	}
	sw, sh := ctx.ScreenSize()
	width, height := guideSize(sw, sh)
	rebuild := name != w.mapName || width != w.width || height != w.height
	if name != w.mapName {
		w.search = map[int]string{}
		w.field = nil
		w.mapImage = nil
		w.mapImageName = ""
		w.mapName = name
		w.resetScroll()
	}
	if w.snapshot != w.service.Snapshot {
		w.snapshot = w.service.Snapshot
		w.dirty = true
	}
	if rebuild {
		w.SetSize(width, height)
		w.SetContent(w.tree())
		w.dirty = true
	}
	if p != w.progress {
		w.progress = p
		w.updateDetails()
	}
	if w.dirty {
		w.refreshPanels()
		w.dirty = false
	}
	if w.statusText != nil {
		w.statusText.Set(w.status())
	}
	w.Publish(ctx)
}
func (w *GuideWindow) Update(ctx Context) bool {
	if !w.IsOpen() {
		return false
	}
	v := w.Window.Update(ctx)
	w.Publish(ctx)
	return v
}
func (w *GuideWindow) resetScroll() {
	w.listScroll = state.NewSignal[float32](0)
	w.detailScroll = state.NewSignal[float32](0)
}
func (w *GuideWindow) tree() widget.Widget {
	if w.search == nil {
		w.search = map[int]string{}
	}
	if w.selected == nil {
		w.selected = map[int]string{}
	}
	if w.listScroll == nil {
		w.resetScroll()
	}
	if w.statusText == nil {
		w.statusText = state.NewSignal(w.status())
	}
	if w.row == nil {
		w.row = state.NewSignal[int](-1)
	}
	w.list = newGuideSlot()
	w.detail = newGuideSlot()
	tabs := []widget.Widget{}
	for i, t := range guideTabs {
		index := i
		tabs = append(tabs, newTabWidget(tabWidgetConfig{label: t.label, active: i == w.tab, width: 90, height: 26, onClick: func() {
			if index == w.tab {
				return
			}
			w.tab = index
			w.field = nil
			w.resetScroll()
			w.SetContent(w.tree())
			w.refreshPanels()
			w.Publish(w.ctx)
		}}))
	}
	tabs = append(tabs, primitives.Expanded(primitives.Box()), rotheme.Button("Refresh", func() { w.service.Refresh(w.serverRoot(), w.serverMode()); w.dirty = true }).Width(70).Height(24))
	if w.field == nil {
		w.field = rotheme.TextField(w.search[w.tab], textfield.TypeText, func(v string) { w.search[w.tab] = v; w.listScroll.Set(0); w.refreshPanels() }, nil, textfield.Placeholder("Search"), textfield.MaxLength(80))
	}
	leftW := max(138, (w.width-30)*38/100)
	bodyH := max(75, w.height-115)
	return Win(Title("Guide"), CloseButton(true), OnClose(w.Close), Size(float32(w.width), float32(w.height)), Content(
		primitives.Box(
			rotheme.Text("Current location: "+w.mapName).MaxLines(1),
			primitives.HBox(tabs...).Gap(3).Height(26),
			primitives.HBox(
				primitives.Box(primitives.Box(w.field).Height(26), primitives.Expanded(w.list)).Width(float32(leftW)).Gap(5),
				primitives.Expanded(w.detail),
			).Height(float32(bodyH)).Gap(10).CrossAlign(primitives.CrossAxisStretch),
			rotheme.Text("").ContentSignal(w.statusText).Color(rotheme.Default.Colors.MutedText).MaxLines(1),
		).Padding(8).Gap(5).CrossAlign(primitives.CrossAxisStretch),
	))
}
func (w *GuideWindow) serverRoot() string {
	v := w.ctx.Config.GuideServerRoot
	if v == "" {
		return "~/rathena"
	}
	return v
}
func (w *GuideWindow) serverMode() string {
	v := w.ctx.Config.GuideMode
	if v == "" {
		return "auto"
	}
	return v
}
func (w *GuideWindow) status() string {
	if w.service.Error != "" {
		return w.service.Error
	}
	if w.service.Loading {
		return "Reading local server metadata…"
	}
	if w.snapshot == nil {
		return "Server metadata unavailable"
	}
	return "Configured snapshot · " + w.snapshot.Loaded.Format("15:04:05") + " · refresh checks every 10s"
}
func (w *GuideWindow) refreshPanels() {
	if w.list == nil {
		return
	}
	w.monsters = nil
	w.npcs = nil
	if w.snapshot != nil {
		if w.tab == 0 {
			w.monsters = w.snapshot.MapMonsters(w.mapName, w.search[0])
		} else {
			w.npcs = w.snapshot.MapNPCs(w.mapName, w.search[1], w.indoor)
		}
	}
	count := len(w.monsters)
	if w.tab == 1 {
		count = len(w.npcs)
	}
	selected := -1
	for i := 0; i < count; i++ {
		if w.key(i) == w.selected[w.tab] {
			selected = i
			break
		}
	}
	if selected < 0 && count > 0 {
		selected = 0
	}
	if selected >= 0 {
		w.selected[w.tab] = w.key(selected)
	} else {
		w.selected[w.tab] = ""
	}
	w.row.Set(selected)
	empty := guideTabs[w.tab].empty
	if w.search[w.tab] != "" {
		empty = "No matching entries"
	}
	if w.snapshot == nil {
		empty = w.status()
	}
	table := rotheme.TableView(rotheme.TableViewColumns([]rotheme.TableViewColumn{{Key: "entry", Flex: 1}}), rotheme.TableViewShowHeader(false), rotheme.TableViewRowCount(count), rotheme.TableViewRowHeight(72), rotheme.TableViewEmptyText(empty), rotheme.TableViewScrollYSignal(w.listScroll), rotheme.TableViewSelectedRow(w.row), rotheme.TableViewBuildCell(func(c rotheme.TableViewCellContext) widget.Widget { return w.listCell(c.Row) }), rotheme.TableViewOnRowClick(func(i int) {
		if i < 0 || i >= count {
			return
		}
		w.selected[w.tab] = w.key(i)
		w.row.Set(i)
		w.detailScroll.Set(0)
		w.updateDetails()
	}))
	w.list.replace(table)
	w.updateDetails()
	invalidateWindowLayout(w.ctx)
	w.Publish(w.ctx)
}
func (w *GuideWindow) key(i int) string {
	if w.tab == 0 {
		return fmt.Sprint(w.monsters[i].ID)
	}
	return w.npcs[i].ID
}
func (w *GuideWindow) listCell(i int) widget.Widget {
	if w.tab == 1 {
		n := w.npcs[i]
		role := n.Role
		if n.Interior {
			role += " · inside building"
		}
		return primitives.Box(rotheme.Text(n.Name).Bold().MaxLines(1), rotheme.Text(role).Color(rotheme.Default.Colors.MutedText).MaxLines(2)).Padding(5).Gap(3)
	}
	m := w.monsters[i]
	sp := w.snapshot.SpawnInfo(w.mapName, m.ID)
	population := 0
	for _, s := range sp {
		population += s.Count
	}
	return primitives.HBox(newGuideImage(w.monsterImage(m.ID), 32, 44), primitives.Expanded(primitives.Box(rotheme.Text(m.Name).Bold().MaxLines(1), rotheme.Text(fmt.Sprintf("Lv.%d · %s %d", m.Level, m.Element, m.ElementLevel)).MaxLines(1), rotheme.Text(fmt.Sprintf("%d configured · %s", population, spawnInterval(sp))).Color(rotheme.Default.Colors.MutedText).MaxLines(1)).Gap(2))).Gap(4).Padding(4)
}
func spawnInterval(a []guide.Spawn) string {
	if len(a) == 0 {
		return "Unavailable"
	}
	first := a[0]
	for _, v := range a {
		if v.Delay != first.Delay || v.Variance != first.Variance {
			return "multiple intervals"
		}
	}
	if first.Delay+first.Variance == 0 {
		return "minimum 1s respawn"
	}
	return fmt.Sprintf("raw %g–%gs", float64(first.Delay)/1000, float64(first.Delay+first.Variance)/1000)
}
func (w *GuideWindow) monsterImage(id int) image.Image {
	if w.previews == nil {
		w.previews = map[int]image.Image{}
	}
	if img, ok := w.previews[id]; ok {
		return img
	}
	if w.preview == nil {
		return nil
	}
	img := w.preview(id, 90, 100)
	w.previews[id] = img
	return img
}
func (w *GuideWindow) indoor(name string) bool {
	return w.ctx.Resources != nil && w.ctx.Resources.IsIndoorMap(name)
}
func (w *GuideWindow) updateDetails() {
	if w.detail == nil {
		return
	}
	rows := []widget.Widget{}
	line := func(t string) { rows = append(rows, rotheme.Text(t)) }
	heading := func(t string) { rows = append(rows, rotheme.Text(t).Bold()) }
	if w.snapshot == nil {
		line(w.status())
		line("Configure [guide] server_root in goro.ini, then Refresh.")
	} else if i := w.row.Get(); i < 0 {
		line(guideTabs[w.tab].empty)
	} else if w.tab == 0 && i < len(w.monsters) {
		m := w.monsters[i]
		heading(m.Name)
		rows = append(rows, newGuideImage(w.monsterImage(m.ID), 90, 100))
		line(fmt.Sprintf("Lv.%d · %s · %s · %s %d", m.Level, m.Race, m.Size, m.Element, m.ElementLevel))
		line(fmt.Sprintf("HP %d · DEF %d · MDEF %d", m.Hp, m.Defense, m.MagicDefense))
		line(fmt.Sprintf("Attack %d / %d", m.Attack, m.Attack2))
		heading("Configured spawns")
		for _, sp := range w.snapshot.SpawnInfo(w.mapName, m.ID) {
			line(guide.SpawnText([]guide.Spawn{sp}))
			lo, hi := w.snapshot.RespawnRange(sp)
			line(fmt.Sprintf("Configured delay after spawn rates: %.3g–%.3gs", float64(lo)/1000, float64(hi)/1000))
			if sp.Level > 0 {
				line(fmt.Sprintf("Spawn level override: %d", sp.Level))
			}
			line(fmt.Sprintf("Area %d,%d ±%d,%d", sp.X, sp.Y, sp.AreaX, sp.AreaY))
		}
		heading("Experience & leveling")
		line(fmt.Sprintf("Base EXP %d · Job EXP %d (solo estimate)", w.snapshot.MapReward(m, w.mapName, false), w.snapshot.MapReward(m, w.mapName, true)))
		line(fmt.Sprintf("Server rates: Base ×%g · Job ×%g", float64(w.snapshot.Rates["base_exp_rate"])/100, float64(w.snapshot.Rates["job_exp_rate"])/100))
		line(fmt.Sprintf("Current class ID %d · Base Lv.%d · Job Lv.%d", w.progress.JobID, w.progress.BaseLevel, w.progress.JobLevel))
		heading("To next level")
		line("Base: " + w.snapshot.Kills(m, w.progress, false))
		line("Job: " + w.snapshot.Kills(m, w.progress, true))
		line("Requirements are the current class thresholds sent by the server.")
		heading("Drops — configured baseline rolls")
		for _, group := range []struct {
			drops []guide.Drop
			mvp   bool
		}{{m.Drops, false}, {m.MvpDrops, true}} {
			for _, d := range group.drops {
				item := w.snapshot.Items[d.Item]
				name := item.Name
				if name == "" {
					name = d.Item
				}
				rate, modifier := w.snapshot.DropRate(m, d, group.mvp)
				value := fmt.Sprintf("%.2f%%", float64(rate)/100)
				if modifier == "Rate unavailable" || modifier == "Caps unavailable" {
					value = modifier
				}
				if group.mvp {
					value += " · MVP prize roll (selection-dependent)"
				}
				rows = append(rows, primitives.HBox(newGuideImage(w.itemIcon(item.ID), 24, 24), primitives.Expanded(primitives.Box(rotheme.Text(name+" · "+value), rotheme.Text(modifier).Color(rotheme.Default.Colors.MutedText)))).Gap(4))
			}
		}
		heading("Skills")
		if len(m.Skills) == 0 {
			line("No configured skills")
		}
		for _, skill := range m.Skills {
			line(skill)
		}
		heading("Scope of these estimates")
		for _, warning := range w.snapshot.Warnings {
			line(warning)
		}
	} else if w.tab == 1 && i < len(w.npcs) {
		n := w.npcs[i]
		heading(n.Name)
		line(n.Role + " · configured NPC")
		if w.snapshot.Indoor[w.mapName] || w.indoor(w.mapName) {
			line("Interior map — location preview hidden")
		} else {
			if n.Interior {
				line("Building entrance (verified warp route)")
			} else {
				line("NPC location")
			}
			if len(n.Entrances) == 0 {
				line("Entrance unavailable")
			} else if img := w.locationImage(n.Entrances); img != nil {
				rows = append(rows, newGuideImage(img, 220, 220))
			} else {
				line("Minimap unavailable")
			}
			for _, p := range n.Entrances {
				line(fmt.Sprintf("%s: %d, %d", w.mapName, p.X, p.Y))
			}
		}
		line("Source: " + n.Source)
		line("Availability may be changed by runtime scripts.")
	}
	content := primitives.Box(rows...).Gap(7).Padding(5).CrossAlign(primitives.CrossAxisStretch)
	w.detail.replace(scrollview.New(content, scrollview.DirectionOpt(scrollview.Vertical), scrollview.ScrollbarOpt(scrollview.ScrollbarAuto), scrollview.ScrollYSignal(w.detailScroll)))
	invalidateWindowLayout(w.ctx)
}
func (w *GuideWindow) itemIcon(id int) image.Image {
	if w.icons == nil {
		w.icons = map[int]image.Image{}
	}
	if img, ok := w.icons[id]; ok {
		return img
	}
	var img image.Image
	if w.ctx.Resources != nil {
		if name, ok := w.ctx.Resources.ItemResourceName(id, true); ok {
			img, _, _ = res.LoadImage(w.ctx.Resources, res.ItemIconTextureCandidates(name))
		}
	}
	w.icons[id] = img
	return img
}
func (w *GuideWindow) locationImage(points []guide.Point) image.Image {
	if w.ctx.Resources == nil || w.ctx.World == nil {
		return nil
	}
	if w.mapImageName != w.mapName {
		w.mapImage, _, _ = res.LoadImage(w.ctx.Resources, minimapImageCandidates(w.mapName))
		w.mapImageName = w.mapName
	}
	if w.mapImage == nil {
		return nil
	}
	mw, mh := minimapWorldSize(w.ctx.World)
	if mw <= 0 || mh <= 0 {
		return nil
	}
	img := scaleWorldMapImage(w.mapImage, image.Pt(220, 220))
	out := image.NewRGBA(img.Bounds())
	for y := 0; y < 220; y++ {
		for x := 0; x < 220; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	for _, p := range points {
		x, y := guideMarker(p, mw, mh, 220, 220)
		for d := -4; d <= 4; d++ {
			for t := -1; t <= 1; t++ {
				out.Set(x+d, y+t, color.RGBA{R: 235, A: 255})
				out.Set(x+t, y+d, color.RGBA{R: 235, A: 255})
			}
		}
	}
	return out
}
func guideMarker(p guide.Point, mw, mh, iw, ih int) (int, int) {
	return min(iw-1, max(0, p.X*iw/mw)), min(ih-1, max(0, (mh-1-p.Y)*ih/mh))
}

// guideSlot replaces only its own subtree, preserving search focus and the
// independent scroll state when selection or character EXP changes.
type guideSlot struct {
	widget.WidgetBase
	child *primitives.BoxWidget
	ctx   widget.Context
}

func newGuideSlot() *guideSlot { s := &guideSlot{}; s.SetVisible(true); s.SetEnabled(true); return s }
func (s *guideSlot) replace(child widget.Widget) {
	if s.child != nil {
		widget.UnmountTree(s.child)
	}
	s.child = primitives.Box(child).CrossAlign(primitives.CrossAxisStretch)
	s.child.SetParent(s)
	if s.ctx != nil {
		widget.MountTree(s.child, s.ctx)
	}
	s.MarkNeedsLayout()
	s.SetNeedsRedraw(true)
}
func (s *guideSlot) Mount(ctx widget.Context) { s.ctx = ctx }
func (s *guideSlot) Unmount()                 { s.ctx = nil }
func (s *guideSlot) Children() []widget.Widget {
	if s.child == nil {
		return nil
	}
	return []widget.Widget{s.child}
}
func (s *guideSlot) Layout(ctx widget.Context, c geometry.Constraints) geometry.Size {
	size := c.BiggestFinite(1, 1)
	if s.child != nil {
		size = widget.LayoutChild(s.child, ctx, c)
		s.child.SetBounds(geometry.FromPointSize(s.Position(), size))
	}
	s.SetBounds(geometry.FromPointSize(s.Position(), size))
	return size
}
func (s *guideSlot) Draw(ctx widget.Context, c widget.Canvas) {
	if s.child != nil {
		s.child.Draw(ctx, c)
	}
}
func (s *guideSlot) Event(ctx widget.Context, e event.Event) bool {
	return s.child != nil && s.child.Event(ctx, e)
}

// Guide thumbnails must scale their pixels as well as their layout bounds.
// The generic static-image widget draws its source at native size.
type guideImageWidget struct {
	widget.WidgetBase
	source, cached image.Image
	width, height  int
}

func newGuideImage(img image.Image, width, height int) *guideImageWidget {
	w := &guideImageWidget{source: img, width: width, height: height}
	w.SetVisible(true)
	w.SetEnabled(false)
	return w
}
func (w *guideImageWidget) Layout(_ widget.Context, c geometry.Constraints) geometry.Size {
	size := c.Constrain(geometry.Sz(float32(w.width), float32(w.height)))
	w.SetBounds(geometry.FromPointSize(w.Position(), size))
	target := image.Pt(max(1, int(size.Width)), max(1, int(size.Height)))
	if w.source != nil && (w.cached == nil || w.cached.Bounds().Size() != target) {
		src := w.source.Bounds()
		dst := image.NewRGBA(image.Rectangle{Max: target})
		if src.Dx() > 0 && src.Dy() > 0 {
			scale := min(float64(target.X)/float64(src.Dx()), float64(target.Y)/float64(src.Dy()))
			width, height := max(1, int(float64(src.Dx())*scale)), max(1, int(float64(src.Dy())*scale))
			x, y := (target.X-width)/2, (target.Y-height)/2
			xdraw.NearestNeighbor.Scale(dst, image.Rect(x, y, x+width, y+height), w.source, src, xdraw.Src, nil)
		}
		w.cached = dst
	}
	return size
}
func (w *guideImageWidget) Draw(_ widget.Context, c widget.Canvas) {
	if w.cached != nil {
		c.DrawImage(w.cached, w.Bounds().Min)
	}
}
func (w *guideImageWidget) Event(widget.Context, event.Event) bool { return false }
