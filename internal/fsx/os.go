package fsx

import (
	"io/fs"
	"os"
)

/* OS is an FS implementation backed by the os package. */
type OS struct{}

/* NewOS returns an os-backed FS. */
func NewOS() *OS { return &OS{} }

/* ReadFile reads the named file. */
func (OS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304 -- FS abstraction intentionally reads caller-supplied paths.
}

/* WriteFile writes data to the named file. */
func (OS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

/* Stat returns file metadata. */
func (OS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

/* MkdirAll creates a directory tree. */
func (OS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }

/* Rename renames or moves a path. */
func (OS) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }

/* Remove deletes a file or empty directory. */
func (OS) Remove(path string) error { return os.Remove(path) }
