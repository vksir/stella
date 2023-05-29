package comm

import (
	"bytes"
	"encoding/base64"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"image"
	"image/png"
	"stella/assets"
	"strings"
)

func ImgEncodeBase64(i image.Image) (string, error) {
	b := bytes.NewBuffer(nil)
	if err := png.Encode(b, i); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

func ImgDecodeBase64(data string) (image.Image, error) {
	i, _, err := image.Decode(strings.NewReader(data))
	return i, err
}

func LoadImg(path string) (image.Image, error) {
	if reader, err := assets.FS.Open(path); err != nil {
		return nil, err
	} else if img, _, err := image.Decode(reader); err != nil {
		return nil, err
	} else {
		return img, nil
	}
}

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
