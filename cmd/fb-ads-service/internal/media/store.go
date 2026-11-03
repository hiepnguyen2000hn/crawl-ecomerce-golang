package media

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Row is a fbads_media row waiting to be downloaded.
type Row struct {
	ID          string
	AdArchiveID string
	JSONPath    string
	Kind        Kind
	URL         string
	ExpiresAt   time.Time
}

// Store is the fbads_media table.
type Store struct {
	DB *sql.DB
}

// UpsertPending records refs as waiting for download. A re-crawled ad brings
// freshly signed URLs: rows that already failed or expired are reset to
// pending with the new URL, rows already downloaded are left alone.
func (s *Store) UpsertPending(ctx context.Context, adArchiveID string, refs []Ref) error {
	if len(refs) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("media: begin: %w", err)
	}
	defer tx.Rollback()

	for _, r := range refs {
		var expires any
		if !r.ExpiresAt.IsZero() {
			expires = r.ExpiresAt
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO fbads_media (ad_archive_id, json_path, kind, source_url, expires_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (ad_archive_id, json_path) DO UPDATE
			SET source_url = EXCLUDED.source_url,
			    expires_at = EXCLUDED.expires_at,
			    kind = EXCLUDED.kind,
			    status = 'pending',
			    attempts = 0,
			    error = NULL,
			    updated_at = now()
			WHERE fbads_media.status <> 'done'`,
			adArchiveID, r.JSONPath, string(r.Kind), r.URL, expires,
		); err != nil {
			return fmt.Errorf("media: upsert %s %s: %w", adArchiveID, r.JSONPath, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("media: commit: %w", err)
	}
	return nil
}

// PendingForJob returns the pending media of every ad the job crawled.
func (s *Store) PendingForJob(ctx context.Context, jobID string) ([]Row, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, ad_archive_id, json_path, kind, source_url, expires_at
		FROM fbads_media
		WHERE status = 'pending'
		  AND ad_archive_id IN (SELECT ad_archive_id FROM fbads_raw WHERE job_id = $1)
		ORDER BY ad_archive_id, json_path`, jobID)
	if err != nil {
		return nil, fmt.Errorf("media: load pending: %w", err)
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var (
			r       Row
			kind    string
			expires sql.NullTime
		)
		if err := rows.Scan(&r.ID, &r.AdArchiveID, &r.JSONPath, &kind, &r.URL, &expires); err != nil {
			return nil, fmt.Errorf("media: scan pending: %w", err)
		}
		r.Kind = Kind(kind)
		if expires.Valid {
			r.ExpiresAt = expires.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MarkDone(ctx context.Context, id, storageKey, contentType string, size int64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE fbads_media
		SET status = 'done', storage_key = $2, content_type = $3, size_bytes = $4,
		    attempts = attempts + 1, error = NULL, updated_at = now()
		WHERE id = $1`, id, storageKey, contentType, size)
	if err != nil {
		return fmt.Errorf("media: mark done %s: %w", id, err)
	}
	return nil
}

// MarkFailed records a failed download. expired means the source URL is
// dead for good (fbcdn answered 403/404/410); only a re-crawl can revive it.
func (s *Store) MarkFailed(ctx context.Context, id string, expired bool, errMsg string) error {
	status := "failed"
	if expired {
		status = "expired"
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE fbads_media
		SET status = $2, error = $3, attempts = attempts + 1, updated_at = now()
		WHERE id = $1`, id, status, errMsg)
	if err != nil {
		return fmt.Errorf("media: mark %s %s: %w", status, id, err)
	}
	return nil
}
