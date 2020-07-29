package memfs

import (
	"fmt"
	"os"
	"strings"

	"github.com/jecoz/flexi/file"
	"github.com/jecoz/flexi/fs"
)

type MemFS struct {
	Root *file.Dir
}

func open(path []string, files []fs.File) fs.File {
	if len(files) == 0 {
		// We do not have anything more to check!
		return nil
	}
	target := ""
	if len(path) > 0 {
		target = path[0]
	}

	for _, v := range files {
		info, err := v.Stat()
		if err != nil || info.Name() != target {
			continue
		}
		if len(path) <= 1 {
			return v
		}

		// Reaching this point means the file is relevant,
		// but we're not yet at the end of the path (on the
		// right way though).
		// What we have in our files list does not matter:
		// the file we're looking for is under this one, hence
		// we should search in the files contained in this
		// directory.
		type hasLS interface {
			LS() []fs.File
		}

		dir, ok := v.(hasLS)
		if !ok {
			// We cannot do anything. The file is supposed to
			// be under this directory, but this is not a
			// directory.
			return nil
		}
		return open(path[1:], dir.LS())
	}
	return nil
}

func (mfs *MemFS) Open(path string) (fs.File, error) {
	fields := strings.Split(path, "/")
	if path == "/" {
		fields = []string{}
	}

	file := open(fields, []fs.File{mfs.Root})
	if file == nil {
		return nil, os.ErrNotExist
	}
	return file, nil
}

func (mfs *MemFS) Create(path string, newfile fs.File) error {
	f, err := mfs.Open(path)
	if err != nil {
		return err
	}
	dir, ok := f.(*file.Dir)
	if !ok {
		return fmt.Errorf("%v is not a directory", path)
	}
	dir.Append(newfile)
	return nil
}

func New(d *file.Dir) *MemFS { return &MemFS{Root: d} }
