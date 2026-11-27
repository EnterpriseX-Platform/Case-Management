package store_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/EnterpriseX-Platform/Case-Management/backend/migrations"
	"github.com/jackc/pgx/v5"
)

// Replicas starting together must not race on the schema (fresh database).
func TestConcurrentMigrate(t *testing.T) {
	url := os.Getenv("CM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	db := fmt.Sprintf("cm_migrate_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	i := strings.LastIndex(url, "/")
	q := ""
	if j := strings.Index(url[i:], "?"); j >= 0 {
		q = url[i+j:]
	}
	fresh := url[:i+1] + db + q

	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for n := 0; n < 6; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pg, err := store.NewPostgres(ctx, fresh, "t")
			if err != nil {
				errs <- err
				return
			}
			defer pg.Close()
			errs <- pg.Migrate(ctx, migrations.All()...)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
