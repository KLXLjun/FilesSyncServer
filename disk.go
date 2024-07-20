package main

import (
	"path"
	"strings"

	"github.com/peterbourgon/diskv/v3"
)

func AdvancedTransformExample(key string) *diskv.PathKey {
	split := strings.Split(key, "/")
	last := len(split) - 1
	return &diskv.PathKey{
		Path:     split[:last],
		FileName: split[last] + ".txt",
	}
}

func InverseTransformExample(pathKey *diskv.PathKey) (key string) {
	txt := pathKey.FileName[len(pathKey.FileName)-5:]
	if txt != ".txt" {
		panic("Invalid file found in storage folder!")
	}
	return strings.Join(pathKey.Path, "/") + pathKey.FileName[:len(pathKey.FileName)-5]
}

func flatTransform(s string) []string {
	return []string{}
}

var DiskCache = diskv.New(diskv.Options{
	BasePath:  path.Join(ExecPath, "database"),
	Transform: flatTransform,
	//AdvancedTransform: AdvancedTransformExample,
	//InverseTransform:  InverseTransformExample,
	CacheSizeMax: 1024 * 1024 * 1024, //1G
})
