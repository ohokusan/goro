package game

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kivutar/goro/client"
	"github.com/kivutar/goro/db"
	"github.com/kivutar/goro/network"
	"github.com/kivutar/goro/render"
	"github.com/kivutar/goro/res"
	"github.com/kivutar/goro/session"
	worldstate "github.com/kivutar/goro/world"
)

// All fixtures are generated geometric pixels, never copied game assets.
func cashMountACTFixture(anchorY int32) []byte {
	var b bytes.Buffer
	put := func(v any) {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			panic(err)
		}
	}
	b.Write([]byte{'A', 'C', 3, 2})
	put(uint16(16))
	b.Write(make([]byte, 10))
	for action := 0; action < 16; action++ {
		put(uint32(4))
		for motion := 0; motion < 4; motion++ {
			b.Write(make([]byte, 32))
			put(uint32(1))
			put(int32(0))
			put(int32(0))
			put(int32(0))
			put(int32(0))
			b.Write([]byte{255, 255, 255, 255})
			put(float32(1))
			put(int32(0))
			put(int32(0))
			put(int32(-1))
			put(int32(1))
			put(int32(0))
			put(int32(action))
			put(anchorY - int32(motion))
			put(int32(0))
		}
	}
	put(int32(0)) // sounds
	for action := 0; action < 16; action++ {
		put(float32(1))
	} // 25 ms
	return b.Bytes()
}

func cashMountSPRFixture() []byte {
	b := []byte{'S', 'P', 1, 1, 1, 0, 2, 0, 2, 0, 1, 1, 1, 1}
	palette := make([]byte, 1024)
	palette[4] = 255
	return append(b, palette...)
}

func writeCashMountResource(t *testing.T, root, name string, data []byte) {
	t.Helper()
	path := filepath.Join(root, strings.ReplaceAll(name, "\\", "/"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func cashMountTestManager(t *testing.T, sex byte) *res.Manager {
	t.Helper()
	root := t.TempDir()
	for _, ext := range []string{"act", "spr"} {
		data := cashMountACTFixture(-15)
		if ext == "spr" {
			data = cashMountSPRFixture()
		}
		writeCashMountResource(t, root, res.PlayerBodyResourceCandidates(0, sex, ext)[0], data)
		writeCashMountResource(t, root, res.PlayerHeadResourceCandidates(0, 1, sex, ext)[0], data)
		writeCashMountResource(t, root, res.PlayerCashMountResourceCandidates(sex, ext)[0], data)
	}
	pal := make([]byte, 1024)
	pal[5] = 255
	writeCashMountResource(t, root, res.PlayerBodyPaletteResourceCandidates(0, sex, 7, "pal")[0], pal)
	writeCashMountResource(t, root, res.PlayerHeadPaletteResourceCandidates(0, 1, sex, 3, "pal")[0], pal)
	return &res.Manager{Root: root}
}

func TestCashMountBodySelectionPaletteAndCache(t *testing.T) {
	for _, sex := range []byte{0, 1} {
		t.Run(res.PlayerSexToken(sex), func(t *testing.T) {
			manager := cashMountTestManager(t, sex)
			character := session.Character{Job: 0, Hair: 1, HeadPal: 3, BodyPal: 7, HeadTop: 10, HeadMid: 11, HeadLow: 12}
			load := func(mounted bool) *humanoidSpriteView {
				t.Helper()
				v, status := loadPlayerHumanoidSpriteView(manager, character, sex, false, mounted)
				if v == nil {
					t.Fatal(status)
				}
				return v
			}
			ordinary := load(false)
			mounted := load(true)
			again := load(true)
			restored := load(false)
			if !mounted.cashMounted || mounted.imf != nil || mounted.body.palette != nil || !strings.Contains(mounted.body.source, "poring_novice_") {
				t.Fatal("mounted body did not use default custom palette and ACT composition")
			}
			if ordinary.body.palette == nil || restored.body.palette != ordinary.body.palette || restored.body.spr != ordinary.body.spr {
				t.Fatal("ordinary body/dye not restored")
			}
			if mounted.head == nil || mounted.head.spr != ordinary.head.spr || mounted.head.palette != ordinary.head.palette || mounted.head.palette == nil {
				t.Fatal("original head or hair color lost")
			}
			if again.body.spr != mounted.body.spr || again.body.act != mounted.body.act || again.body == mounted.body {
				t.Fatal("parsed cache or independent view state incorrect")
			}
			if character.BodyPal != 7 || character.HeadTop != 10 || character.HeadMid != 11 || character.HeadLow != 12 {
				t.Fatal("source appearance mutated")
			}
			base := actorSpriteKey{job: 0, sex: sex, bodyPalette: 7}
			cash := base
			cash.cashMounted = true
			views := map[actorSpriteKey]*humanoidSpriteView{base: ordinary, cash: mounted}
			ctx := client.Context{Session: &session.Session{AccountID: 100}}
			mode := &WorldMode{actorViews: views}
			actor := worldstate.Actor{ID: 300, Job: 0, Sex: sex, BodyPal: 7, CashMounted: true}
			if mode.humanoidSpriteViewForActor(ctx, actor) != mounted {
				t.Fatal("mounted lookup missed cache variant")
			}
			actor.CashMounted = false
			if mode.humanoidSpriteViewForActor(ctx, actor) != ordinary {
				t.Fatal("ordinary cache polluted by mount")
			}
		})
	}
}

func TestCashMountFallback(t *testing.T) {
	for _, problem := range []string{"missing-act", "missing-spr", "bad-act", "bad-spr", "empty-actions", "missing-anchor", "bad-frame"} {
		t.Run(problem, func(t *testing.T) {
			manager := cashMountTestManager(t, 1)
			actPath := res.PlayerCashMountResourceCandidates(1, "act")[0]
			sprPath := res.PlayerCashMountResourceCandidates(1, "spr")[0]
			switch problem {
			case "missing-act", "missing-spr":
				path := actPath
				if problem == "missing-spr" {
					path = sprPath
				}
				if err := os.Remove(filepath.Join(manager.Root, strings.ReplaceAll(path, "\\", "/"))); err != nil {
					t.Fatal(err)
				}
			case "bad-act":
				writeCashMountResource(t, manager.Root, actPath, []byte("broken"))
			case "bad-spr":
				writeCashMountResource(t, manager.Root, sprPath, []byte("broken"))
			case "empty-actions":
				b := make([]byte, 16)
				copy(b, []byte{'A', 'C', 3, 2})
				b = append(b, 0, 0, 0, 0)
				writeCashMountResource(t, manager.Root, actPath, b)
			case "missing-anchor", "bad-frame":
				act, _, err := manager.LoadACT([]string{actPath})
				if err != nil {
					t.Fatal(err)
				}
				if problem == "missing-anchor" {
					act.Actions[0].Animations[0].Pos = nil
				} else {
					act.Actions[0].Animations[0].Layers[0].Index = 999
				}
			}
			v, status := loadPlayerHumanoidSpriteView(manager, session.Character{Hair: 1, BodyPal: 7}, 1, false, true)
			if v == nil || v.cashMounted || v.body.palette == nil || strings.Contains(v.body.source, "poring_novice_") || !strings.Contains(status, "cash-mount-fallback") {
				t.Fatalf("fallback failed: %s", status)
			}
		})
	}
	manager := cashMountTestManager(t, 1)
	v, status := loadPlayerHumanoidSpriteView(manager, session.Character{Job: 1, Hair: 1}, 1, false, true)
	if v == nil || v.cashMounted || strings.Contains(status, "cash-mount-fallback") {
		t.Fatalf("non-Novice affected: %s", status)
	}
}

func cashMountStatusPacket(id uint32, active bool) network.Packet {
	data := make([]byte, 9)
	binary.LittleEndian.PutUint16(data, 0x0196)
	binary.LittleEndian.PutUint16(data[2:], 613)
	binary.LittleEndian.PutUint32(data[4:], id)
	if active {
		data[8] = 1
	}
	return network.Packet{ID: 0x0196, Data: data}
}

func TestCashMount2008StatusTransitions(t *testing.T) {
	manager := cashMountTestManager(t, 1)
	s := &session.Session{AccountID: 100, CharID: 200, Sex: 1, Selected: session.Character{ID: 200, Hair: 1, BodyPal: 7}}
	ctx := client.Context{Session: s, World: worldstate.New(), Resources: manager}
	ctx.World.Player.ID = 100
	ctx.World.UpsertActor(worldstate.Actor{ID: 300, Appearance: true, Job: 0, Head: 1, HeadTop: 8, BodyPal: 7})
	mode := &WorldMode{}
	for _, id := range []uint32{100, 300} {
		for _, active := range []bool{true, true, false, true, false} {
			change, ok, err := network.ParseStatusEffectChange(cashMountStatusPacket(id, active))
			if !ok || err != nil {
				t.Fatal(err)
			}
			mode.applyStatusEffectChange(ctx, change)
			if id == 100 {
				if localPlayerCashMounted(ctx) != active || mode.playerView == nil || mode.playerView.cashMounted != active {
					t.Fatalf("local transition active=%t not rendered immediately", active)
				}
				if active && mode.playerView.body.palette != nil || !active && mode.playerView.body.palette == nil {
					t.Fatal("local body palette not switched")
				}
				if s.Selected.BodyPal != 7 || s.Selected.Hair != 1 {
					t.Fatal("local appearance changed")
				}
			} else {
				// Full actor/option snapshots must preserve independent EFST state.
				ctx.World.UpsertActor(worldstate.Actor{ID: 300, Job: 0, Appearance: true, Head: 1, HeadTop: 8, BodyPal: 7, HasState: true})
				actor := ctx.World.Actors[300]
				if actor.CashMounted != active || actor.BodyPal != 7 || actor.HeadTop != 8 {
					t.Fatal("remote mount or appearance lost on snapshot")
				}
			}
		}
	}
	mode.applyStatusEffectChange(ctx, network.StatusEffectChange{StatusID: db.StatusAllRiding, ActorID: 300, Active: true})
	ctx.World.RemoveActor(300)
	ctx.World.UpsertActor(worldstate.Actor{ID: 300})
	if ctx.World.Actors[300].CashMounted {
		t.Fatal("mount leaked into reused actor ID")
	}
}

func TestCashMountStatusDuringLogin(t *testing.T) {
	ctx := client.Context{Session: &session.Session{AccountID: 100, CharID: 200}, World: worldstate.New()}
	mode := &LoginMode{}
	for _, active := range []bool{true, false} {
		if !mode.applyLoginStatusEffectPacket(ctx, cashMountStatusPacket(100, active)) || localPlayerCashMounted(ctx) != active {
			t.Fatal("login did not retain cash mount status")
		}
	}
}

func TestCashMountAnchorsHeadgearAndEightDirections(t *testing.T) {
	manager := cashMountTestManager(t, 1)
	view, status := loadPlayerHumanoidSpriteView(manager, session.Character{Hair: 1}, 1, false, true)
	if view == nil {
		t.Fatal(status)
	}
	// Give the independent head and headgear single green/blue pixels at
	// distinct attachment offsets, so rendering checks detect missing layers.
	makeAttached := func(c color.RGBA, anchorY int32) *spriteView {
		v := &spriteView{act: &res.ACT{Actions: make([]res.ACTAction, 16)}, spr: &res.SPR{IndexedCount: 1, RGBAIndex: 1, Frames: []res.SPRFrame{{Width: 2, Height: 2, Data: []byte{1, 1, 1, 1}}}}, images: make(map[spriteFrameKey]*render.Image)}
		v.spr.Palette[1] = [4]byte{c.R, c.G, c.B, 255}
		for i := range v.act.Actions {
			v.act.Actions[i] = res.ACTAction{Animations: []res.ACTAnimation{{Pos: []res.ACTPosition{{Y: anchorY}}, Layers: []res.ACTLayer{{Index: 0, ScaleX: 1, ScaleY: 1, Color: [4]float32{1, 1, 1, 1}}}}}}
		}
		return v
	}
	view.head = makeAttached(color.RGBA{G: 255, A: 255}, 0)
	view.accessoryTop = makeAttached(color.RGBA{B: 255, A: 255}, 5)
	for _, family := range []int{spriteActionIdle, spriteActionWalk} {
		for dir := 0; dir < 8; dir++ {
			for motion := 0; motion < 4; motion++ {
				action := view.body.act.Actions[family*8+dir]
				b, ok := composeHumanoidBillboard(view, family, dir, action, motion, 0)
				if !ok {
					t.Fatal("no composed billboard")
				}
				// World-relative pixel positions survive billboard cropping.
				x := int(b.anchorX) + family*8 + dir
				y := int(b.anchorY) - 15 - motion
				if b.image.RGBA().RGBAAt(x, y).G != 255 || b.image.RGBA().RGBAAt(x, y-5).B != 255 {
					t.Fatalf("head/headgear anchor mismatch family=%d dir=%d motion=%d", family, dir, motion)
				}
			}
		}
	}
	view.started = time.Unix(100, 0)
	_, ok := humanoidBillboardForState(view, spriteState{headTurn: true, actionFamily: spriteActionIdle}, view.started.Add(30*time.Millisecond))
	if !ok {
		t.Fatal("idle did not render")
	}
	found := false
	for key := range view.billboards {
		found = found || key.bodyMotion == 1
	}
	if !found {
		t.Fatal("head turn froze mounted idle animation")
	}
	view.body.act.Actions[0].Animations[0].Layers[0].ScaleX = float32(math.NaN())
	if validateCashMountBody(view.body) == nil {
		t.Fatal("non-finite conversion accepted")
	}
}

// Opt-in structural/composition check; does not establish in-game correctness.
func TestCashMountSuppliedAssets(t *testing.T) {
	manager := realDataManager(t)
	for _, sex := range []byte{0, 1} {
		view, status := loadPlayerHumanoidSpriteView(manager, session.Character{Hair: 1, BodyPal: 7}, sex, false, true)
		if view == nil || !view.cashMounted || view.head == nil {
			t.Fatalf("sex=%d: %s", sex, status)
		}
		t.Logf("sex=%d ACT=%d.%d actions=%d SPR=%d.%d frames=%d", sex, view.body.act.VersionMajor, view.body.act.VersionMinor, len(view.body.act.Actions), view.body.spr.VersionMajor, view.body.spr.VersionMinor, len(view.body.spr.Frames))
		for family := 0; family < 2; family++ {
			for dir := 0; dir < 8; dir++ {
				action := view.body.act.Actions[family*8+dir]
				for motion := range action.Animations {
					state := spriteState{actionFamily: family, direction: (4 - dir) & 7, headTurn: true, hasFixedMotion: true, fixedMotion: motion}
					if _, ok := humanoidBillboardForState(view, state, time.Now()); !ok {
						t.Fatalf("sex=%d family=%d direction=%d motion=%d failed", sex, family, dir, motion)
					}
				}
			}
		}
	}
}
