package flexi

import (
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/jecoz/flexi/file"
	"github.com/jecoz/flexi/file/memfs"
	"github.com/jecoz/flexi/fs"
	"github.com/jecoz/flexi/styx"
)

type Srv struct {
	Mtpt string
	Ln   net.Listener
	Spawner
	FS fs.FS

	pool *intPool
}

func (s *Srv) Serve() error {
	return styx.Serve(s.Ln, s.FS)
}

func (s *Srv) NewRemote(name string, done func(*Remote)) (r *Remote, err error) {
	r, err = NewRemote(s.Mtpt, name, s.Spawner)
	if err != nil {
		return
	}
	r.Done = done
	return
}

func (s *Srv) RestoreRemote(f fs.File) (*Remote, error) {
	return nil, errors.New("restore remote: not implemented yet")
}

func ServeFlexi(ln net.Listener, mtpt string, s Spawner) error {
	srv := &Srv{Mtpt: mtpt, Ln: ln, Spawner: s, pool: newIntPool()}
	clone := file.WithRead("clone", func(p []byte) (int, error) {
		// Users read the clone file to obtain
		// a new remote process.
		i := srv.pool.Get()
		name := strconv.FormatInt(i, 10)
		s := []byte(name)
		if len(s) > len(p) {
			srv.pool.Put(i)
			return 0, io.ErrShortBuffer
		}

		remote, err := srv.NewRemote(name, func(r *Remote) {
			// When the remote is deleted, return its
			// index to the pool.
			srv.pool.Put(i)
		})
		if err != nil {
			srv.pool.Put(i)
			return 0, err
		}

		srv.FS.Create("", remote)
		return copy(p, s), io.EOF
	})
	// Restore previous list of remotes found in mtpt.
	// Each remote, when spawned
	oldremotes := file.DiskLS(mtpt)()
	remotes := make([]*Remote, 0, len(oldremotes))
	for i, v := range oldremotes {
		restored, err := srv.RestoreRemote(v)
		if err != nil {
			log.Printf("error * restore failed (%d): %v", i, err)
			continue
		}
		remotes = append(remotes, restored)
	}
	log.Printf("*** %d remotes restored from %v", len(remotes), mtpt)

	files := append(make([]fs.File, 0, len(remotes)+1), clone)
	for _, v := range remotes {
		files = append(files, v)
	}
	root := file.NewDirFiles("", files...)
	srv.FS = memfs.New(root)

	return srv.Serve()
}

type intPool struct {
	n    int64
	pool sync.Pool
}

func newIntPool() (p *intPool) {
	p = new(intPool)
	p.pool = sync.Pool{
		New: func() interface{} {
			defer func() { p.n++ }()
			return p.n
		},
	}
	return
}

func (p *intPool) Get() int64  { return p.pool.Get().(int64) }
func (p *intPool) Put(i int64) { p.pool.Put(i) }
