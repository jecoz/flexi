// SPDX-FileCopyrightText: 2020 jecoz
//
// SPDX-License-Identifier: BSD-3-Clause

package file

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

type Plumber struct {
	f    func(*Plumber) bool
	name string

	sync.Mutex
	buf     *LimitBuffer
	plumbed bool
	modTime time.Time
}

func (p *Plumber) Size() int64 {
	p.Lock()
	defer p.Unlock()
	return p.buf.Size()
}
func (p *Plumber) Open() (io.ReadWriteCloser, error) { return p, nil }
func (p *Plumber) Stat() (os.FileInfo, error) {
	p.Lock()
	defer p.Unlock()
	return Info{
		name:    p.name,
		size:    p.buf.Size(),
		mode:    0222,
		modTime: p.modTime,
		isDir:   false,
	}, nil
}

func (p *Plumber) Read(b []byte) (int, error) {
	p.Lock()
	defer p.Unlock()
	// Read is only called from the inside to obtain
	// buffer's contents, usually only after Close
	// is called (hence to writes occur).
	return p.buf.Read(b)
}

func (p *Plumber) Write(b []byte) (int, error) {
	p.Lock()
	defer p.Unlock()
	if p.plumbed {
		// We've plumbed successfully already.
		// Write is no longer allowed.
		return 0, errors.New("plumbed already")
	}
	return p.buf.Write(b)
}

func (p *Plumber) Close() error {
	p.Lock()

	if p.plumbed {
		p.Unlock()
		return nil
	}
	if err := p.buf.Close(); err != nil {
		p.Unlock()
		return err
	}

	// Plumb only if there is a plumbed function
	// & the buffer contains some data.
	if p.f == nil {
		p.Unlock()
		return nil
	}
	p.Unlock()

	// Note that from this point on the p is unlocked.
	if p.Size() > 0 {
		p.plumbed = p.f(p)
	}
	return nil
}

func NewPlumber(name string, f func(*Plumber) bool) *Plumber {
	return &Plumber{name: name, f: f, buf: &LimitBuffer{}, modTime: time.Now()}
}
