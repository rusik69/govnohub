package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/ssh"
)

var (
	ErrInvalidSSHKey   = errors.New("invalid SSH public key")
	ErrDuplicateSSHKey = errors.New("SSH key already in use")
)

type SSHKeyInfo struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

func parseSSHPublicKey(raw string) (ssh.PublicKey, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", ErrInvalidSSHKey
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return nil, "", ErrInvalidSSHKey
	}
	return pub, ssh.FingerprintSHA256(pub), nil
}

func (s *Service) AddSSHKey(ctx context.Context, userID uuid.UUID, title, key string) (*SSHKeyInfo, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	pub, fingerprint, err := parseSSHPublicKey(key)
	if err != nil {
		return nil, err
	}
	stored := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))

	var info SSHKeyInfo
	err = s.pool.QueryRow(ctx, `
		INSERT INTO user_ssh_keys (user_id, title, public_key, fingerprint)
		VALUES ($1, $2, $3, $4)
		RETURNING id, title, fingerprint, created_at`,
		userID, title, stored, fingerprint,
	).Scan(&info.ID, &info.Title, &info.Fingerprint, &info.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, ErrDuplicateSSHKey
		}
		return nil, err
	}
	return &info, nil
}

func (s *Service) ListSSHKeys(ctx context.Context, userID uuid.UUID) ([]SSHKeyInfo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, fingerprint, created_at
		FROM user_ssh_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SSHKeyInfo
	for rows.Next() {
		var k SSHKeyInfo
		if err := rows.Scan(&k.ID, &k.Title, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	if out == nil {
		out = []SSHKeyInfo{}
	}
	return out, rows.Err()
}

func (s *Service) DeleteSSHKey(ctx context.Context, userID, keyID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_ssh_keys WHERE id=$1 AND user_id=$2`, keyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *Service) LookupUserBySSHPublicKey(ctx context.Context, pub ssh.PublicKey) (uuid.UUID, string, error) {
	fingerprint := ssh.FingerprintSHA256(pub)
	var userID uuid.UUID
	var username string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username
		FROM user_ssh_keys k
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
