// SPDX-FileCopyrightText: 2020 jecoz
//
// SPDX-License-Identifier: BSD-3-Clause

package file

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jecoz/flexi/fs"
)

type Dir struct {
	name string
	perm os.FileMode

	sync.Mutex
	ls      func() []fs.File
	modTime time.Time
}

func (d *Dir) Ls() []fs.File {
	d.Lock()
	defer d.Unlock()
	return d.ls()
}

func (d *Dir) Close() error {
	d.Lock()
	defer d.Unlock()
	d.modTime = time.Now()
	d.ls = func() []fs.File {
		return []fs.File{}
	}
	return nil
}

func (d *Dir) Open() (io.ReadWriteCloser, error) { return &dirReader{dir: d}, nil }
func (d *Dir) Stat() (os.FileInfo, error) {
	d.Lock()
	defer d.Unlock()
	return Info{
		name:    d.name,
		mode:    d.perm | os.ModeDir,
		modTime: d.modTime,
		isDir:   true,
		size:    0,
	}, nil
}

func (d *Dir) Find(name string) (fs.File, error) {
	if name == d.name {
		return d, nil
	}
	filename := strings.TrimPrefix(name, d.name)

	for _, v := range d.ls() {
		info, err := v.Stat()
		if err != nil {
			continue
		}
		if info.Name() == filename {
			return v, nil
		}
	}
	return nil, os.ErrNotExist
}

func (d *Dir) Append(f fs.File) {
	d.Lock()
	defer d.Unlock()
	d.modTime = time.Now()
	oldls := d.ls
	d.ls = func() []fs.File {
		return append(oldls(), f)
	}
}

func (d *Dir) Remove(f fs.File) {
	d.Lock()
	defer d.Unlock()
	d.modTime = time.Now()
	oldls := d.ls
	d.ls = func() []fs.File {
		files := oldls()
		filtered := make([]fs.File, 0, len(files))
		for _, v := range files {
			if v != f {
				filtered = append(filtered, v)
			}
		}
		return filtered
	}
}

// LsDisk returns an ls function that inspects path on disk. Basically
// it works just like Unix's ls command, but returns a list of File.
// It can be used to create Dir instances that act on the disk.
func LsDisk(path string) func() []fs.File {
	return func() []fs.File {
		dir, err := os.Open(path)
		if err != nil {
			return []fs.File{}
		}
		defer dir.Close()

		// Even though Readdir might return an error, it will
		// return the FileInfos found till that point. That's
		// enough for our use-case.
		infos, _ := dir.Readdir(-1)
		files := make([]fs.File, len(infos))
		for i, v := range infos {
			child := filepath.Join(path, v.Name())
			if v.IsDir() {
				files[i] = &Dir{
					name:    v.Name(),
					perm:    v.Mode(),
					modTime: v.ModTime(),
					ls:      LsDisk(child),
				}
			} else {
				files[i] = NewRegular(child, v)
			}
		}
		return files
	}
}

func NewDirLs(name string, ls func() []fs.File) *Dir {
	return &Dir{
		perm:    os.ModePerm,
		name:    name,
		modTime: time.Now(),
		ls:      ls,
	}
}

func NewDirFiles(name string, files ...fs.File) *Dir {
	return NewDirLs(name, func() []fs.File {
		return files
	})
}

type dirReader struct {
	dir    *Dir
	offset int
}

var errNotSupported = errors.New("not supported")

func (d *dirReader) Read(p []byte) (int, error)  { return 0, errNotSupported }
func (d *dirReader) Write(p []byte) (int, error) { return 0, errNotSupported }
func (d *dirReader) Close() error                { return nil }

func (d *dirReader) Readdir(n int) ([]os.FileInfo, error) {
	if d.dir == nil {
		return nil, os.ErrInvalid
	}
	d.dir.Lock()
	all := d.dir.ls()
	d.dir.Unlock()

	if d.offset >= len(all) {
		return nil, io.EOF
	}
	files := all[d.offset:]
	count := len(files)
	take := n
	if take <= 0 || take > count {
		take = count
	}

	files = files[:take]
	fis := make([]os.FileInfo, len(files))
	for i, v := range files {
		info, err := v.Stat()
		if err != nil {
			return nil, err
		}
		fis[i] = info
	}
	d.offset += take
	return fis, nil
}
