package fanzhiyi

import (
	"errors"
	"github.com/fogleman/gg"
	"image"
	"image/color"
	"stella/internal/comm"
	"strings"
)

const (
	ImgPath    = "assets/img/fan_zhi_yi.png"
	FontPath   = "assets/font/msyh.ttf"
	FontWeight = 54
)

func getImg(text string) (image.Image, error) {
	img, err := comm.LoadImg(ImgPath)
	if err != nil {
		return nil, err
	}
	dc := gg.NewContextForImage(img)
	dc.SetColor(color.White)
	if fontFace, err := comm.LoadFont(FontPath, FontWeight); err != nil {
		return nil, err
	} else {
		dc.SetFontFace(fontFace)
	}
	maxWidth := float64(dc.Width()) * 0.9
	text += " 没这个能力知道吗？"
	textSlice := textWrap(dc, text, maxWidth)
	if len(textSlice) > 2 {
		return nil, errors.New("text too long")
	}
	text = strings.Join(textWrap(dc, text, maxWidth), "\n")
	dc.DrawStringWrapped(text, float64(dc.Width())*0.5, float64(dc.Height())*0.85, 0.5, 0.5, maxWidth, 1.5, gg.AlignCenter)
	return dc.Image(), nil
}

func getImgBase64(text string) (string, error) {
	img, err := getImg(text)
	if err != nil {
		return "", err
	}
	data, err := comm.ImgEncodeBase64(img)
	if err != nil {
		return "", err
	}
	return data, nil
}

func textWrap(dc *gg.Context, text string, width float64) []string {
	textSlice := []string{""}
	for _, word := range strings.Split(text, "") {
		line := textSlice[len(textSlice)-1]
		w, _ := dc.MeasureString(line + word)
		if w <= width {
			textSlice[len(textSlice)-1] = line + word
		} else {
			textSlice = append(textSlice, word)
		}
	}
	return textSlice
}
