package ui

import (
	"image"
	"strings"
	"testing"

	uiapp "github.com/gogpu/ui/app"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/uitest"
	"github.com/gogpu/ui/widget"
	"github.com/kivutar/goro/client"
	"github.com/kivutar/goro/guide"
	"github.com/kivutar/goro/input"
	"github.com/kivutar/goro/session"
)

func guideTexts(w widget.Widget) string {
	if w == nil {
		return ""
	}
	s := ""
	if t, ok := w.(*primitives.TextWidget); ok {
		s = t.Content() + "\n"
	}
	for _, c := range w.Children() {
		s += guideTexts(c)
	}
	return s
}
func guideTestWindow(t *testing.T) (*GuideWindow, *uiapp.App, client.Context) {
	t.Helper()
	a := uiapp.New()
	manager := NewManager()
	bridge := mailWindowTestApp{basicMenuTestApp{app: a}}
	manager.SetUIApp(bridge)
	ctx := client.Context{UIApp: bridge, Input: input.NewState(), Session: &session.Session{}, UIManager: manager, ScreenW: 800, ScreenH: 600}
	w := &GuideWindow{}
	w.Toggle(ctx)
	w.mapName = "field"
	w.snapshot = &guide.Snapshot{Monsters: map[int]guide.Monster{1: {ID: 1, Name: "First", Level: 1}, 2: {ID: 2, Name: "Second", Level: 2}}, Spawns: []guide.Spawn{{Map: "field", MobID: 1}, {Map: "field", MobID: 2}}, NPCs: []guide.NPC{{ID: "one", Map: "field", Name: "Outdoor", X: 10, Y: 20}, {ID: "inside", Map: "inside", Name: "Vendor", Role: "Shop"}}, Warps: []guide.Warp{{Map: "field", Destination: "inside", X: 30, Y: 40}}, Indoor: map[string]bool{"inside": true}}
	w.SetContent(w.tree())
	w.refreshPanels()
	a.Frame()
	a.Window().DrawTo(&uitest.MockCanvas{})
	return w, a, ctx
}
func TestGuideSelectionSearchTabsAndMapChange(t *testing.T) {
	w, a, _ := guideTestWindow(t)
	if w.selected[0] != "1" || !strings.Contains(guideTexts(w.detail), "First") {
		t.Fatal("first selection")
	}
	field := w.field
	w.selected[0] = "2"
	w.refreshPanels()
	if w.field != field || !strings.Contains(guideTexts(w.detail), "Second") {
		t.Fatal("selection replaced search or detail")
	}
	w.tab = 1
	w.field = nil
	w.SetContent(w.tree())
	w.refreshPanels()
	if w.selected[1] != "one" || len(w.npcs) != 2 {
		t.Fatal("NPC first selection")
	}
	if !strings.Contains(guideTexts(w.detail), "Minimap unavailable") {
		t.Fatal("missing minimap fallback")
	}
	w.tab = 0
	w.SetContent(w.tree())
	w.refreshPanels()
	if w.selected[0] != "2" {
		t.Fatal("selection lost on tab switch")
	}
	w.search[0] = "First"
	w.refreshPanels()
	if len(w.monsters) != 1 || w.selected[0] != "1" {
		t.Fatal("search")
	}
	w.mapName = "empty"
	w.refreshPanels()
	if len(w.monsters) != 0 || w.selected[0] != "" || !strings.Contains(guideTexts(w.detail), "No monsters on the map") {
		t.Fatal("stale content")
	}
	a.Frame()
	a.Window().DrawTo(&uitest.MockCanvas{})
}
func TestGuideIndoorHidesCoordinatesAndPreview(t *testing.T) {
	w, _, _ := guideTestWindow(t)
	w.tab = 1
	w.mapName = "inside"
	w.SetContent(w.tree())
	w.refreshPanels()
	s := guideTexts(w.detail)
	if !strings.Contains(s, "Interior map") || strings.Contains(s, "Minimap") || strings.Contains(s, "10, 20") {
		t.Fatal(s)
	}
}
func TestGuideSmallLayoutAndMarker(t *testing.T) {
	for _, size := range []geometry.Size{geometry.Sz(800, 600), geometry.Sz(640, 480), geometry.Sz(400, 300)} {
		w, a, ctx := guideTestWindow(t)
		ctx.ScreenW = int(size.Width)
		ctx.ScreenH = int(size.Height)
		width, height := guideSize(ctx.ScreenW, ctx.ScreenH)
		w.SetSize(width, height)
		w.SetContent(w.tree())
		w.refreshPanels()
		a.Frame()
		a.Window().DrawTo(&uitest.MockCanvas{})
		if width >= ctx.ScreenW || height >= ctx.ScreenH {
			t.Fatal("offscreen")
		}
		if w.list.Bounds().Width() <= 0 || w.detail.Bounds().Width() <= 0 {
			t.Fatalf("not two columns: %+v %+v", w.list.Bounds(), w.detail.Bounds())
		}
	}
	x, y := guideMarker(guide.Point{X: 100, Y: 300}, 400, 400, 200, 200)
	if x != 50 || y != 49 {
		t.Fatalf("marker %d,%d", x, y)
	}
}
func TestGuidePreviewAndMissingIconCache(t *testing.T) {
	w, _, _ := guideTestWindow(t)
	calls := 0
	w.previews = nil
	w.preview = func(int, int, int) image.Image { calls++; return nil }
	w.monsterImage(1)
	w.monsterImage(1)
	if calls != 1 {
		t.Fatal("missing sprite not cached")
	}
	if w.itemIcon(501) != nil {
		t.Fatal("missing icon")
	}
}

func TestGuidePublishedRowClick(t *testing.T) {
	w, a, _ := guideTestWindow(t)
	p := geometry.Pt(0, 0)
	var node widget.Widget = w.list
	for node != nil {
		if b, ok := node.(interface{ Bounds() geometry.Rect }); ok {
			p = p.Add(b.Bounds().Min)
		}
		parent, ok := node.(interface{ Parent() widget.Widget })
		if !ok {
			break
		}
		node = parent.Parent()
	}
	p.X += 50
	p.Y += 72 + 20
	a.Window().HandleEvent(uitest.Click(p.X, p.Y))
	a.Window().HandleEvent(uitest.Release(p.X, p.Y))
	if w.selected[0] != "2" {
		t.Fatalf("published row click at %v selected %s", p, w.selected[0])
	}
	a.Frame()
	a.Window().DrawTo(&uitest.MockCanvas{})
}

func TestGuideImagePixelsFitLayoutBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 90, 100))
	w := newGuideImage(src, 32, 44)
	size := w.Layout(widget.NewContext(), geometry.Tight(geometry.Sz(20, 30)))
	if size != geometry.Sz(20, 30) || w.cached.Bounds().Size() != image.Pt(20, 30) {
		t.Fatal("thumbnail can overflow its row")
	}
}
