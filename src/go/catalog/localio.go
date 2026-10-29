package catalog

import (
	"context"
	"io/fs"
	"net/url"
	"strings"

	icebergio "github.com/apache/iceberg-go/io"
)

// literalFS is the local file IO for file:// locations. iceberg-go's LocalFS
// percent-decodes them, so a data file whose partition value is escaped in
// its path (stream_edid's ':' as %3A) is written under a directory holding
// the raw ':' while the manifest records %3A, and readers that take paths
// literally (pyiceberg, Java) cannot find it. literalFS uses a location's
// path as written, as iceberg-go's S3 IO does with object keys.
type literalFS struct{ local icebergio.LocalFS }

func init() {
	icebergio.Unregister("file")
	icebergio.Register("file", func(context.Context, *url.URL, map[string]string) (icebergio.IO, error) {
		return literalFS{}, nil
	})
}

// literal is location without its file:// scheme, and not unescaped.
func literal(location string) string {
	return strings.TrimPrefix(location, "file://")
}

func (l literalFS) Open(name string) (icebergio.File, error) { return l.local.Open(literal(name)) }
func (l literalFS) ReadFile(name string) ([]byte, error)     { return l.local.ReadFile(literal(name)) }
func (l literalFS) Create(name string) (icebergio.FileWriter, error) {
	return l.local.Create(literal(name))
}
func (l literalFS) WriteFile(name string, p []byte) error { return l.local.WriteFile(literal(name), p) }
func (l literalFS) Remove(name string) error              { return l.local.Remove(literal(name)) }
func (l literalFS) RemoveAll(name string) error           { return l.local.RemoveAll(literal(name)) }
func (l literalFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	return l.local.WalkDir(literal(root), fn)
}
func (l literalFS) ReadDir(name string) ([]fs.DirEntry, error) { return l.local.ReadDir(literal(name)) }
func (l literalFS) MkdirAll(name string) error                 { return l.local.MkdirAll(literal(name)) }
func (l literalFS) Mkdir(name string) error                    { return l.local.Mkdir(literal(name)) }
func (l literalFS) Stat(name string) (fs.FileInfo, error)      { return l.local.Stat(literal(name)) }
func (l literalFS) Rename(oldpath, newpath string) error {
	return l.local.Rename(literal(oldpath), literal(newpath))
}
func (l literalFS) RenameNoReplace(oldpath, newpath string) error {
	return l.local.RenameNoReplace(literal(oldpath), literal(newpath))
}
