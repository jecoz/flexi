// SPDX-FileCopyrightText: 2020 jecoz
//
// SPDX-License-Identifier: BSD-3-Clause

package flexi

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jecoz/flexi/file"
	"github.com/jecoz/flexi/fs"
)

type Remote struct {
	*file.Dir
	S    Spawner
	Name string
	Done func()

	mtpt string
	proc *RemoteProcess
}

func (r *Remote) Close() error {
	if r.proc != nil {
		mtpt := filepath.Join(r.mtpt, r.Name)
		if err := Umount(mtpt); err != nil {
			return fmt.Errorf("unable to umount %v: %w", mtpt, err)
		}
		if err := r.S.Kill(context.Background(), r.proc.SpawnedReader()); err != nil {
			return err
		}
	}
	r.Dir = file.NewDirFiles("")
	if r.Done != nil {
		r.Done()
	}
	return nil
}

func Mount(addr, mtpt string) error {
	return mount(addr, mtpt)
}

func Umount(path string) error {
	if err := umount(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func (r *Remote) mirrorRemoteProcess(ctx context.Context, path string, i *Stdio, id int) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Prepare output encoding helpers. If this is the behaviour
	// of every flexi process, we could add one more helper layer.

	h := NewProcessHelper(i, 6)
	defer h.Done()
	herr := func(format string, args ...interface{}) {
		h.Errf(format, args...)
	}

	h.Progress(1, "spawning remote process")
	rp, err := r.S.Spawn(ctx, i.In, id)
	if err != nil {
		herr("spawn remote process: %w", err)
		return
	}
	h.Progress(2, "remote process spawned @ %v", rp.Addr)

	// From now on we also need to remove the spawned
	// process in case of error to avoid resource leaks.
	oldherr := herr
	herr = func(format string, args ...interface{}) {
		r.S.Kill(ctx, rp.SpawnedReader())
		oldherr(format, args...)
	}

	if err := Mount(rp.Addr, path); err != nil {
		herr("mount remote process: %w", err)
		return
	}
	h.Progress(3, "remote process mounted @ %v", path)

	oldherr = herr
	herr = func(format string, args ...interface{}) {
		exec.CommandContext(ctx, "umount", path).Run()
		os.RemoveAll(path)
		oldherr(format, args...)
	}

	h.Progress(4, "storing spawn information at %v", path)

	// TODO: try creating a version of this function that can
	// detect when it is not possible to create the file in the
	// remote namespace w/o leaking goroutines nor locking.
	spawned, err := os.Create(filepath.Join(path, "spawned"))
	if err != nil {
		herr("create back file: %w", err)
		return
	}
	defer spawned.Close()

	if _, err := io.Copy(spawned, rp.SpawnedReader()); err != nil {
		herr("copying spawn information: %w", err)
		return
	}
	r.proc = rp
	h.Progress(5, "remote process info encoded & saved")
}

func RestoreRemote(mtpt string, name string, s Spawner, rp *RemoteProcess) (*Remote, error) {
	// In contrast with NewRemote, we're not killing anything
	// here even though we could.
	path := filepath.Join(mtpt, name)
	if err := Mount(rp.Addr, path); err != nil {
		return nil, err
	}

	// We assume we're restoring a spawned remote. If
	// that is the case, there is no need for creating
	// the err, state and spawn files, as they belong
	// to the past. If this wasn't a spawned remote,
	// users should just delete this and create a new one.

	mirror := file.NewDirLs("mirror", file.LsDisk(path))
	return &Remote{
		mtpt: mtpt,
		S:    s,
		Name: name,
		Dir:  file.NewDirFiles(name, mirror),
		proc: rp,
	}, nil
}

func NewRemote(mtpt string, name string, s Spawner, id int) (*Remote, error) {
	// First check that the file is not present already.
	// In that case, it means this remote should've been
	// restored instead, or might be. Anyway it **might**
	// not be treated as an error in the future.
	path := filepath.Join(mtpt, name)
	if len(file.LsDisk(path)()) > 0 {
		return nil, fmt.Errorf("remote exists already at %v", path)
	}
	os.RemoveAll(path)

	r := &Remote{mtpt: mtpt, S: s, Name: name}
	errfile := file.NewMulti("err")
	statefile := file.NewMulti("state")
	spawn := file.NewPlumber("spawn", func(p *file.Plumber) bool {
		go func() {
			defer errfile.Close()
			defer statefile.Close()

			r.mirrorRemoteProcess(context.Background(), path, &Stdio{
				In:    p,
				Err:   errfile,
				State: statefile,
			}, id)
		}()
		return true
	})
	static := []fs.File{spawn, errfile, statefile}
	mirror := file.NewDirLs("mirror", file.LsDisk(path))
	r.Dir = file.NewDirFiles(name, append(static, mirror)...)
	return r, nil
}
