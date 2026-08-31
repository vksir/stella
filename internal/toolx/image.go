package toolx

// 图片格式检测。

var pngSignature = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// detectSupportedImageMimeType 检测支持的图片 MIME 类型，不支持返回空串。
func detectSupportedImageMimeType(buffer []byte) string {
	if startsWith(buffer, []byte{0xff, 0xd8, 0xff}) {
		if buffer[3] == 0xf7 {
			return ""
		}
		return "image/jpeg"
	}
	if startsWith(buffer, pngSignature) {
		if isPng(buffer) && !isAnimatedPng(buffer) {
			return "image/png"
		}
		return ""
	}
	if startsWithAscii(buffer, 0, "GIF") {
		return "image/gif"
	}
	if startsWithAscii(buffer, 0, "RIFF") && startsWithAscii(buffer, 8, "WEBP") {
		return "image/webp"
	}
	if startsWithAscii(buffer, 0, "BM") && isBmp(buffer) {
		return "image/bmp"
	}
	return ""
}

func isPng(buffer []byte) bool {
	return len(buffer) >= 16 &&
		readUint32BE(buffer, len(pngSignature)) == 13 &&
		startsWithAscii(buffer, 12, "IHDR")
}

// isAnimatedPng 判断是否为 APNG：IHDR 之后、IDAT 之前出现 acTL 块。
func isAnimatedPng(buffer []byte) bool {
	offset := len(pngSignature)
	for offset+8 <= len(buffer) {
		chunkLength := readUint32BE(buffer, offset)
		chunkTypeOffset := offset + 4
		if startsWithAscii(buffer, chunkTypeOffset, "acTL") {
			return true
		}
		if startsWithAscii(buffer, chunkTypeOffset, "IDAT") {
			return false
		}
		nextOffset := offset + 8 + int(chunkLength) + 4
		if nextOffset <= offset || nextOffset > len(buffer) {
			return false
		}
		offset = nextOffset
	}
	return false
}

func isBmp(buffer []byte) bool {
	if len(buffer) < 26 {
		return false
	}
	declaredFileSize := readUint32LE(buffer, 2)
	pixelDataOffset := readUint32LE(buffer, 10)
	dibHeaderSize := readUint32LE(buffer, 14)
	if declaredFileSize != 0 && declaredFileSize < 26 {
		return false
	}
	if pixelDataOffset < 14+dibHeaderSize {
		return false
	}
	if declaredFileSize != 0 && pixelDataOffset >= declaredFileSize {
		return false
	}

	var colorPlanes, bitsPerPixel uint16
	if dibHeaderSize == 12 {
		colorPlanes = readUint16LE(buffer, 22)
		bitsPerPixel = readUint16LE(buffer, 24)
	} else if dibHeaderSize >= 40 && dibHeaderSize <= 124 {
		if len(buffer) < 30 {
			return false
		}
		colorPlanes = readUint16LE(buffer, 26)
		bitsPerPixel = readUint16LE(buffer, 28)
	} else {
		return false
	}
	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
		return colorPlanes == 1
	}
	return false
}

func readUint16LE(buffer []byte, offset int) uint16 {
	return uint16(buffer[offset]) | uint16(buffer[offset+1])<<8
}

func readUint32BE(buffer []byte, offset int) uint32 {
	return uint32(buffer[offset])<<24 | uint32(buffer[offset+1])<<16 |
		uint32(buffer[offset+2])<<8 | uint32(buffer[offset+3])
}

func readUint32LE(buffer []byte, offset int) uint32 {
	return uint32(buffer[offset]) | uint32(buffer[offset+1])<<8 |
		uint32(buffer[offset+2])<<16 | uint32(buffer[offset+3])<<24
}

func startsWith(buffer, prefix []byte) bool {
	if len(buffer) < len(prefix) {
		return false
	}
	for i := range prefix {
		if buffer[i] != prefix[i] {
			return false
		}
	}
	return true
}

func startsWithAscii(buffer []byte, offset int, text string) bool {
	if len(buffer) < offset+len(text) {
		return false
	}
	for i := 0; i < len(text); i++ {
		if buffer[offset+i] != text[i] {
			return false
		}
	}
	return true
}
