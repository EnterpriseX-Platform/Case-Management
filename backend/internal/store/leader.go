package store

import (
	"context"
	"hash/fnv"
	"time"
)

// Leader is implemented by stores that can elect one active instance of a
// singleton service (the scheduler) among several replicas.
type Leader interface {
	// Lead blocks until this process holds the named lock or ctx ends, then
	// runs fn with a context that is cancelled if the lock is lost. It
	// returns when fn returns or the lock is lost; callers loop to re-elect.
	Lead(ctx context.Context, name string, fn func(ctx context.Context)) error
}

// Lead in memory: a single process is always the leader.
func (m *Memory) Lead(ctx context.Context, _ string, fn func(ctx context.Context)) error {
	fn(ctx)
	return nil
}

func lockKey(tenant, name string) int64 {
	h := fnv.New64a()
	h.Write([]byte("cm-leader:" + tenant + ":" + name))
	return int64(h.Sum64())
}

// Lead uses a Postgres session advisory lock held on a dedicated
// connection: if the process or its connection dies, Postgres releases the
// lock and a standby replica takes over within a few seconds.
func (p *Postgres) Lead(ctx context.Context, name string, fn func(ctx context.Context)) error {
	key := lockKey(p.Tenant, name)
	for {
		conn, err := p.pool.Acquire(ctx)
		if err != nil {
			return err
		}
		var got bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
			conn.Release()
			return err
		}
		if !got {
			conn.Release()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
				continue
			}
		}
		lctx, cancel := context.WithCancel(ctx)
		watching := make(chan struct{})
		// watch the connection: losing it means losing the lock
		go func() {
			defer close(watching)
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-lctx.Done():
					return
				case <-t.C:
					if _, err := conn.Exec(lctx, "SELECT 1"); err != nil && lctx.Err() == nil {
						cancel()
						return
					}
				}
			}
		}()
		fn(lctx)
		cancel()
		<-watching // the connection is not shared between goroutines
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key)
		conn.Release()
		return nil
	}
}
