package app

// Background jobs for large volumes: daily summary for reports and moving
// long-closed cases to the archive tables.

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

func (s *Service) reader() store.Store {
	if s.R != nil {
		return s.R
	}
	return s.S
}

// RunRollup refreshes the daily summary for the last two days.
func (s *Service) RunRollup(ctx context.Context) (int, error) {
	sc, ok := s.S.(store.Scaler)
	if !ok {
		return 0, nil
	}
	return 0, sc.RollupDaily(ctx, s.Now().AddDate(0, 0, -2))
}

// RebuildRollup recomputes the whole summary (after an upgrade or import).
func (s *Service) RebuildRollup(ctx context.Context) error {
	if sc, ok := s.S.(store.Scaler); ok {
		return sc.RollupDaily(ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	return nil
}

// RunArchive moves cases closed more than CM_ARCHIVE_AFTER_DAYS days ago
// (default 0 = off) to the archive tables, in batches.
func (s *Service) RunArchive(ctx context.Context) (int, error) {
	days, _ := strconv.Atoi(os.Getenv("CM_ARCHIVE_AFTER_DAYS"))
	sc, ok := s.S.(store.Scaler)
	if days <= 0 || !ok {
		return 0, nil
	}
	n, err := sc.ArchiveCases(ctx, s.Now().AddDate(0, 0, -days), 500)
	if n > 0 {
		Metrics.Archived.Add(int64(n))
	}
	return n, err
}
