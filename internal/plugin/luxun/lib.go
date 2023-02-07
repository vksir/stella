package luxun

import (
	"errors"
	"github.com/fogleman/gg"
	"image"
	"image/color"
	"qq-bot-go/pkg/util/fontutil"
	"qq-bot-go/pkg/util/imgutil"
	"strings"
)

const (
	ImgPath    = "asserts/img/lu_xun.jpg"
	FontPath   = "asserts/font/msyh.ttf"
	FontWeight = 34
)

func getImg(text string) (image.Image, error) {
	img, err := imgutil.ReadImage("asserts/img/lu_xun.jpg")
	if err != nil {
		return nil, err
	}
	dc := gg.NewContextForImage(img)
	dc.SetColor(color.White)
	if fontFace, err := fontutil.LoadFont("asserts/font/msyh.ttf", FontWeight); err != nil {
		return nil, err
	} else {
		dc.SetFontFace(fontFace)
	}
	maxWidth := float64(dc.Width()) * 0.9
	textSlice := textWrap(dc, text, maxWidth)
	if len(textSlice) > 2 {
		return nil, errors.New("text too long")
	}
	text = strings.Join(textWrap(dc, text, maxWidth), "\n")
	dc.DrawStringWrapped(text, float64(dc.Width())*0.5, float64(dc.Height())*0.65, 0.5, 0.5, maxWidth, 2, gg.AlignCenter)
	dc.DrawStringAnchored("——鲁迅", float64(dc.Width())*0.75, float64(dc.Height())*0.85, 0.5, 0.5)
	return dc.Image(), nil
}

func getImgBase64(text string) (string, error) {
	img, err := getImg(text)
	if err != nil {
		return "", err
	}
	data, err := imgutil.EncodeBase64(img)
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
