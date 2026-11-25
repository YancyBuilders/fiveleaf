package main

import (
	"path/filepath"
	"slices"
	"strings"
)

type pathTreeRow[T any] struct {
	dir, name string
	e         T
	depth     int
}

func pathTree[T any](files []T, root string, pathOf func(T) string, closed func(string) bool) []pathTreeRow[T] {
	type folder struct {
		name, path string
		dirs       []*folder
		files      []T
	}
	top := &folder{path: root}
	byPath := map[string]*folder{root: top}
	var dirOf func(string) *folder
	dirOf = func(path string) *folder {
		if found := byPath[path]; found != nil {
			return found
		}
		up := filepath.Dir(path)
		if up == path {
			return top
		}
		parent := dirOf(up)
		child := &folder{name: filepath.Base(path), path: path}
		parent.dirs = append(parent.dirs, child)
		byPath[path] = child
		return child
	}
	for _, file := range files {
		dir := filepath.Dir(pathOf(file))
		if dir != root && !inFolder(root, dir) {
			dir = root
		}
		parent := dirOf(dir)
		parent.files = append(parent.files, file)
	}
	var rows []pathTreeRow[T]
	var add func(*folder, int)
	add = func(parent *folder, depth int) {
		slices.SortFunc(parent.dirs, func(left, right *folder) int { return strings.Compare(left.name, right.name) })
		for _, sub := range parent.dirs {
			name := sub.name
			for len(sub.dirs) == 1 && len(sub.files) == 0 {
				sub = sub.dirs[0]
				name += "/" + sub.name
			}
			rows = append(rows, pathTreeRow[T]{dir: sub.path, name: name, depth: depth})
			if closed == nil || !closed(sub.path) {
				add(sub, depth+1)
			}
		}
		for _, file := range parent.files {
			rows = append(rows, pathTreeRow[T]{e: file, depth: depth})
		}
	}
	add(top, 0)
	return rows
}
