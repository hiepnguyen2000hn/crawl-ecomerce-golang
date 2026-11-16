package pipiads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"
)

var (
	ErrNoAccountsAvailable = errors.New("pipiads: no live accounts with available credits")
)

type Account struct {
	ID           string    `json:"id,omitempty"`
	Email        string    `json:"email"`
	Password     string    `json:"password"`
	AccessToken  string    `json:"access_token"`
	DeviceID     string    `json:"device_id"`
	Proxy        string    `json:"proxy"`
	TotalCredits int       `json:"total_credits"`
	UsedCredits  int       `json:"used_credits"`
	Dead         bool      `json:"dead"`
	LastUsedAt   time.Time `json:"last_used_at,omitempty"`
}

type AccountRegistry struct {
	db     *sql.DB
	client Client
	mu     sync.Mutex
}

func NewAccountRegistry(db *sql.DB, client Client) *AccountRegistry {
	return &AccountRegistry{
		db:     db,
		client: client,
	}
}

// SyncAccounts loads accounts from a JSON file (e.g. pipiads_accounts.json) into the DB
func (r *AccountRegistry) SyncAccounts(ctx context.Context, filePath string) error {
	if filePath == "" {
		return nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("pipiads: read accounts file: %w", err)
	}

	var rawAccounts []struct {
		Email       string            `json:"email"`
		Password    string            `json:"password"`
		Proxy       string            `json:"proxy"`
		Cookies     map[string]string `json:"cookies"`
		Success     bool              `json:"success"`
		AccessToken string            `json:"access_token"`
		DeviceID    string            `json:"device_id"`
	}

	if err := json.Unmarshal(data, &rawAccounts); err != nil {
		return fmt.Errorf("pipiads: parse accounts json: %w", err)
	}

	for _, raw := range rawAccounts {
		if raw.Email == "" || raw.Password == "" {
			continue
		}
		token := raw.AccessToken
		deviceID := raw.DeviceID

		// Try extracting token and device_id from cookies if available
		if token == "" && raw.Cookies != nil {
			if uInfo, ok := raw.Cookies["PP-userInfo"]; ok {
				var info struct {
					AccessToken string `json:"access_token"`
				}
				// uInfo might be url-encoded or raw JSON
				if unescaped, err := url.QueryUnescape(uInfo); err == nil {
					_ = json.Unmarshal([]byte(unescaped), &info)
				}
				if info.AccessToken == "" {
					_ = json.Unmarshal([]byte(uInfo), &info)
				}
				token = info.AccessToken
			}
		}
		if deviceID == "" {
			deviceID = "1317542974" // default virtual device ID
		}

		query := `
			INSERT INTO pipiads_accounts (email, password, access_token, device_id, proxy, total_credits, used_credits, dead)
			VALUES ($1, $2, $3, $4, $5, 500, 0, false)
			ON CONFLICT (email) DO UPDATE SET
				password = EXCLUDED.password,
				access_token = CASE WHEN EXCLUDED.access_token != '' THEN EXCLUDED.access_token ELSE pipiads_accounts.access_token END,
				proxy = CASE WHEN EXCLUDED.proxy != '' THEN EXCLUDED.proxy ELSE pipiads_accounts.proxy END;
		`
		_, err := r.db.ExecContext(ctx, query, raw.Email, raw.Password, token, deviceID, raw.Proxy)
		if err != nil {
			return fmt.Errorf("pipiads: insert account %s: %w", raw.Email, err)
		}
	}

	return nil
}

// AcquireAccount selects an alive account with available credits, prioritized by least recently used.
func (r *AccountRegistry) AcquireAccount(ctx context.Context) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		SELECT id, email, password, access_token, device_id, proxy, total_credits, used_credits, dead, last_used_at
		FROM pipiads_accounts
		WHERE dead = false AND (total_credits - used_credits) > 0
		ORDER BY last_used_at ASC NULLS FIRST
		LIMIT 1;
	`

	var a Account
	var lastUsed sql.NullTime
	err := r.db.QueryRowContext(ctx, query).Scan(
		&a.ID,
		&a.Email,
		&a.Password,
		&a.AccessToken,
		&a.DeviceID,
		&a.Proxy,
		&a.TotalCredits,
		&a.UsedCredits,
		&a.Dead,
		&lastUsed,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAccountsAvailable
		}
		return nil, fmt.Errorf("pipiads: query acquire account: %w", err)
	}

	if lastUsed.Valid {
		a.LastUsedAt = lastUsed.Time
	}

	// Update last_used_at
	_, _ = r.db.ExecContext(ctx, `UPDATE pipiads_accounts SET last_used_at = now() WHERE id = $1`, a.ID)

	return &a, nil
}

// SyncAccountQuota fetches current quota from PiPiAds and updates database
func (r *AccountRegistry) SyncAccountQuota(ctx context.Context, a *Account) error {
	usage, err := r.client.GetMemberUsage(ctx, a)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			_ = r.MarkAccountDead(ctx, a.ID, true)
		}
		return err
	}

	a.TotalCredits = usage.TotalCredits
	a.UsedCredits = usage.UsedCredits

	query := `
		UPDATE pipiads_accounts
		SET total_credits = $1, used_credits = $2, last_used_at = now()
		WHERE id = $3;
	`
	_, err = r.db.ExecContext(ctx, query, a.TotalCredits, a.UsedCredits, a.ID)
	return err
}

// MarkAccountDead sets dead status when account is banned or token invalidated
func (r *AccountRegistry) MarkAccountDead(ctx context.Context, id string, dead bool) error {
	query := `UPDATE pipiads_accounts SET dead = $1 WHERE id = $2;`
	_, err := r.db.ExecContext(ctx, query, dead, id)
	return err
}

// RecordCreditUsage updates used_credits in DB
func (r *AccountRegistry) RecordCreditUsage(ctx context.Context, id string, usedIncrement int) error {
	query := `
		UPDATE pipiads_accounts
		SET used_credits = used_credits + $1, last_used_at = now()
		WHERE id = $2;
	`
	_, err := r.db.ExecContext(ctx, query, usedIncrement, id)
	return err
}
