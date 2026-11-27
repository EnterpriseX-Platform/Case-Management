package store_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/EnterpriseX-Platform/Case-Management/backend/migrations"
)

func testStore(t *testing.T, tenant string) store.Store {
	url := os.Getenv("CM_TEST_DATABASE_URL")
	if url == "" {
		return store.NewMemory()
	}
	pg, err := store.NewPostgres(context.Background(), url, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.Migrate(context.Background(), migrations.All()...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	return pg
}

// Each worker kind only claims jobs of its own queue.
func TestLeaseJobsByQueue(t *testing.T) {
	ctx := context.Background()
	s := testStore(t, "t-"+id.New())
	now := time.Now()
	for _, q := range []string{store.QueueEngine, store.QueueIntegration, ""} {
		j := &store.JobRec{ID: id.New(), Queue: q, Job: engine.Job{InstanceID: "i", TokenID: id.New(), Node: "n"},
			CaseID: "c", Status: store.JobPending, MaxAttempts: 3, NextRunAt: now.Add(-time.Second), CreatedAt: now}
		if err := s.PutJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := s.LeaseJobs(ctx, "w1", []string{store.QueueEngine}, now, time.Minute, 10)
	if err != nil || len(eng) != 1 || eng[0].Queue != store.QueueEngine {
		t.Fatalf("engine lease: %d jobs, err %v", len(eng), err)
	}
	integ, _ := s.LeaseJobs(ctx, "w2", []string{store.QueueIntegration}, now, time.Minute, 10)
	if len(integ) != 2 { // explicit integration + legacy job without a queue
		t.Fatalf("integration lease: %d jobs", len(integ))
	}
	if rest, _ := s.LeaseJobs(ctx, "w3", nil, now, time.Minute, 10); len(rest) != 0 {
		t.Fatalf("jobs leased twice: %d", len(rest))
	}
}

// Only one replica leads; when it stops, a standby takes over.
func TestLeaderFailover(t *testing.T) {
	if os.Getenv("CM_TEST_DATABASE_URL") == "" {
		t.Skip("CM_TEST_DATABASE_URL not set")
	}
	tenant := "t-" + id.New()
	a, b := testStore(t, tenant).(store.Leader), testStore(t, tenant).(store.Leader)
	var active atomic.Int32
	var bRan atomic.Bool
	ctxA, stopA := context.WithCancel(context.Background())
	aLeading := make(chan struct{})
	go a.Lead(ctxA, "scheduler", func(ctx context.Context) {
		active.Add(1)
		close(aLeading)
		<-ctx.Done()
		active.Add(-1)
	})
	<-aLeading
	ctxB, stopB := context.WithTimeout(context.Background(), 20*time.Second)
	defer stopB()
	bDone := make(chan struct{})
	go func() {
		b.Lead(ctxB, "scheduler", func(ctx context.Context) {
			if active.Add(1) != 1 {
				t.Error("two leaders at once")
			}
			bRan.Store(true)
			active.Add(-1)
		})
		close(bDone)
	}()
	time.Sleep(4 * time.Second) // B keeps trying while A leads
	if bRan.Load() {
		t.Fatal("standby ran while the leader was active")
	}
	stopA()
	select {
	case <-bDone:
	case <-time.After(15 * time.Second):
		t.Fatal("standby did not take over")
	}
	if !bRan.Load() {
		t.Fatal("standby did not lead")
	}
}
