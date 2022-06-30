package asserts

import (
	"embed"
	"io/fs"
)

//go:embed asserts/*
var asserts embed.FS

func Open(path string) (fs.File, error) {
	return asserts.Open(path)
}

func ReadFile(path string) ([]byte, error) {
	return asserts.ReadFile(path)
}
