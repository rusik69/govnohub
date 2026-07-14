package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/openpgp"
)

var (
	ErrInvalidGPGKey   = errors.New("invalid GPG public key")
	ErrDuplicateGPGKey = errors.New("GPG key already in use")
)

// GPGKeyInfo holds information about a stored GPG public key.
type GPGKeyInfo struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

// parseGPGPublicKey parses an armored GPG public key and returns the key ID/fingerprint.
func parseGPGPublicKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidGPGKey
	}
	el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidGPGKey, err)
	}
	if len(el) == 0 || el[0] == nil {
		return "", ErrInvalidGPGKey
	}
	// Use the primary identity's key fingerprint
	fingerprint := fmt.Sprintf("%016X", el[0].PrimaryKey.Fingerprint[12:])
	if fingerprint == "" {
		return "", ErrInvalidGPGKey
	}
	return fingerprint, nil
}

// AddGPGKey stores a new GPG public key for the user.
func (s *Service) AddGPGKey(ctx context.Context, userID uuid.UUID, title, key string) (*GPGKeyInfo, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	fingerprint, err := parseGPGPublicKey(key)
	if err != nil {
		return nil, err
	}

	var info GPGKeyInfo
	err = s.pool.QueryRow(ctx, `
		INSERT INTO user_gpg_keys (user_id, title, public_key, fingerprint)
		VALUES ($1, $2, $3, $4)
		RETURNING id, title, fingerprint, created_at`,
		userID, title, key, fingerprint,
	).Scan(&info.ID, &info.Title, &info.Fingerprint, &info.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, ErrDuplicateGPGKey
		}
		return nil, err
	}
	return &info, nil
}

// ListGPGKeys returns all GPG keys for a user.
func (s *Service) ListGPGKeys(ctx context.Context, userID uuid.UUID) ([]GPGKeyInfo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, fingerprint, created_at
		FROM user_gpg_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GPGKeyInfo
	for rows.Next() {
		var k GPGKeyInfo
		if err := rows.Scan(&k.ID, &k.Title, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	if out == nil {
		out = []GPGKeyInfo{}
	}
	return out, rows.Err()
}

// DeleteGPGKey removes a GPG key by ID, scoped to the user.
func (s *Service) DeleteGPGKey(ctx context.Context, userID, keyID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_gpg_keys WHERE id=$1 AND user_id=$2`, keyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// LookupUserByGPGKeyFingerprint finds a user by their GPG key fingerprint.
func (s *Service) LookupUserByGPGKeyFingerprint(ctx context.Context, fingerprint string) (uuid.UUID, string, error) {
	var userID uuid.UUID
	var username string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username
		FROM user_gpg_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.fingerprint = $1`, fingerprint,
	).Scan(&userID, &username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", ErrUnauthorized
		}
		return uuid.Nil, "", err
	}
	return userID, username, nil
}
