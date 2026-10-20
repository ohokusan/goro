package game

import (
	"image"

	"github.com/kivutar/goro/client"
)

func (m *WorldMode) presentGuide(ctx client.Context) {
	m.ui.guideWindow.Present(ctx, func(id, width, height int) image.Image {
		if id <= 0 || id > 65535 {
			return nil
		}
		return m.monsterInfoPreviewImage(ctx, uint16(id), width, height)
	})
}
