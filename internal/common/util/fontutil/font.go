package fontutil

import (
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"qq-bot-go/assets"
)

func LoadFont(path string, points float64) (font.Face, error) {
	fontBytes, err := assets.FS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}
	face := truetype.NewFace(f, &truetype.Options{
		Size: points,
	})
	return face, nil
}
