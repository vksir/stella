package tool

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/gif"
)

const (
	imageMaxWidth    = 2000
	imageMaxHeight   = 2000
	imageMaxBytes    = 4_718_592 // base64 编码后的字节数
	imageMaxPixels   = 80_000_000
	imageJpegQuality = 80
)

func processImage(data []byte, mimeType string, autoResize bool) ImageProcessorResult {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > imageMaxPixels {
		return omittedImage("could not be decoded or exceeds the image pixel limit")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return omittedImage("could not be converted to a supported inline image format")
	}
	needsConversion := mimeType == "image/bmp"
	if !needsConversion && (!autoResize || (config.Width <= imageMaxWidth && config.Height <= imageMaxHeight && base64Size(len(data)) < imageMaxBytes)) {
		return ImageProcessorResult{OK: true, Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType}
	}
	if !autoResize {
		var output bytes.Buffer
		if err := png.Encode(&output, decoded); err != nil {
			return omittedImage("could not be converted to a supported inline image format")
		}
		return ImageProcessorResult{
			OK: true, Data: base64.StdEncoding.EncodeToString(output.Bytes()), MimeType: "image/png",
			Hints: []string{"[Image converted from image/bmp to image/png.]"},
		}
	}

	width, height := config.Width, config.Height
	if width > imageMaxWidth {
		height = max(1, int(math.Round(float64(height)*float64(imageMaxWidth)/float64(width))))
		width = imageMaxWidth
	}
	if height > imageMaxHeight {
		width = max(1, int(math.Round(float64(width)*float64(imageMaxHeight)/float64(height))))
		height = imageMaxHeight
	}
	for {
		resized := decoded
		if width != config.Width || height != config.Height {
			canvas := image.NewRGBA(image.Rect(0, 0, width, height))
			draw.CatmullRom.Scale(canvas, canvas.Bounds(), decoded, decoded.Bounds(), draw.Src, nil)
			resized = canvas
		}
		for _, candidate := range []struct {
			mimeType string
			encode   func(*bytes.Buffer) error
		}{
			{"image/png", func(out *bytes.Buffer) error { return png.Encode(out, resized) }},
			{"image/jpeg", func(out *bytes.Buffer) error {
				return jpeg.Encode(out, resized, &jpeg.Options{Quality: imageJpegQuality})
			}},
			{"image/jpeg", func(out *bytes.Buffer) error { return jpeg.Encode(out, resized, &jpeg.Options{Quality: 70}) }},
			{"image/jpeg", func(out *bytes.Buffer) error { return jpeg.Encode(out, resized, &jpeg.Options{Quality: 55}) }},
			{"image/jpeg", func(out *bytes.Buffer) error { return jpeg.Encode(out, resized, &jpeg.Options{Quality: 40}) }},
		} {
			var output bytes.Buffer
			if err := candidate.encode(&output); err != nil || base64Size(output.Len()) >= imageMaxBytes {
				continue
			}
			var hints []string
			if needsConversion {
				hints = append(hints, fmt.Sprintf("[Image converted from image/bmp to %s.]", candidate.mimeType))
			}
			if width != config.Width || height != config.Height {
				hints = append(hints, fmt.Sprintf("[Image: original %dx%d, displayed at %dx%d. Multiply coordinates by %.2f to map to original image.]",
					config.Width, config.Height, width, height, float64(config.Width)/float64(width)))
			}
			return ImageProcessorResult{
				OK: true, Data: base64.StdEncoding.EncodeToString(output.Bytes()), MimeType: candidate.mimeType, Hints: hints,
			}
		}
		if width == 1 && height == 1 {
			break
		}
		width = max(1, int(math.Floor(float64(width)*0.75)))
		height = max(1, int(math.Floor(float64(height)*0.75)))
	}
	return omittedImage("could not be resized below the inline image size limit")
}

func supportedInlineImageType(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

func base64Size(bytes int) int {
	return (bytes + 2) / 3 * 4
}

func omittedImage(reason string) ImageProcessorResult {
	return ImageProcessorResult{Message: "[Image omitted: " + strings.TrimSuffix(reason, ".") + ".]"}
}
