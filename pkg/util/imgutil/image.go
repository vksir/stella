package imgutil

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"qq-bot-go/asserts"
	"strings"
)

func EncodeBase64(i image.Image) (string, error) {
	b := bytes.NewBuffer(nil)
	if err := png.Encode(b, i); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

func DecodeBase64(data string) (image.Image, error) {
	i, _, err := image.Decode(strings.NewReader(data))
	return i, err
}

func ReadImage(path string) (image.Image, error) {
	if reader, err := asserts.FS.Open(path); err != nil {
		return nil, err
	} else if img, _, err := image.Decode(reader); err != nil {
		return nil, err
	} else {
		return img, nil
	}
}
