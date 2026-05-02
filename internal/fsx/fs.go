// Package fsx defines the filesystem seam for pgsync.
package fsx

import "io/fs"

/* FS abstracts filesystem operations used by pgsync. */
type FS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
	MkdirAll(path string, perm fs.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
}
