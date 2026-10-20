package game

import (
	"fmt"
	"math"

	"github.com/kivutar/goro/res"
)

func loadCashMountOrBodySpriteView(manager *res.Manager, appearance humanoidAppearance, label string) (*spriteView, string, bool) {
	mountStatus := ""
	if appearance.cashMounted && appearance.job == 0 {
		// nil palette keeps the custom SPR's default colors, regardless of dye.
		body, status := loadSpriteView(manager, res.PlayerCashMountResourceCandidates(appearance.sex, "act"), res.PlayerCashMountResourceCandidates(appearance.sex, "spr"), nil, label+" cash mount")
		if body != nil {
			if err := validateCashMountBody(body); err == nil {
				return body, status, true
			} else {
				status += " invalid=" + err.Error()
			}
		}
		mountStatus = " cash-mount-fallback: " + status
	}
	body, status := loadBodySpriteView(manager, appearance.job, appearance.sex, appearance.bodyPalette, appearance.admin, label+" body")
	return body, status + mountStatus, false
}

// Reject incomplete experimental conversions instead of rendering a partial
// body or losing the head. ACT 2.3 stores attachment points in Animation.Pos.
func validateCashMountBody(body *spriteView) error {
	if body == nil || body.act == nil || body.spr == nil || !actFitsSPR(body.act, body.spr) {
		return fmt.Errorf("ACT/SPR frame references do not match")
	}
	for _, action := range body.act.Actions {
		for _, anim := range action.Animations {
			for _, layer := range anim.Layers {
				if layer.SPRType != res.SPRFramePalette && layer.SPRType != res.SPRFrameRGBA || math.IsNaN(float64(layer.ScaleX)) || math.IsInf(float64(layer.ScaleX), 0) || math.IsNaN(float64(layer.ScaleY)) || math.IsInf(float64(layer.ScaleY), 0) {
					return fmt.Errorf("invalid layer type or scale")
				}
			}
		}
	}
	for actionIndex := 0; actionIndex < 16; actionIndex++ {
		if actionIndex >= len(body.act.Actions) || len(body.act.Actions[actionIndex].Animations) == 0 {
			return fmt.Errorf("idle/walk action %d missing", actionIndex)
		}
		for motion, anim := range body.act.Actions[actionIndex].Animations {
			if len(anim.Pos) == 0 {
				return fmt.Errorf("action %d motion %d head anchor missing", actionIndex, motion)
			}
			visible := false
			for _, layer := range anim.Layers {
				if layer.Index >= 0 {
					w, h, ok := spriteLayerFrameSize(body, layer.Index, layer.SPRType)
					visible = visible || ok && w > 0 && h > 0 && layer.Color[3] > 0 && layer.ScaleX != 0 && layer.ScaleY != 0
				}
			}
			if !visible {
				return fmt.Errorf("action %d motion %d has no visible body", actionIndex, motion)
			}
		}
	}
	return nil
}
