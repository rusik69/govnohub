package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrUserExists         = errors.New("user already exists")
	ErrInsufficientScope  = errors.New("insufficient token scope")
	ErrAccountLocked      = errors.New("account locked due to too many failed login attempts")
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	ScopeRepo      = "repo"
	ScopeRepoWrite = "repo:write"
	ScopeWorkflow  = "workflow"
	ScopeReadUser  = "read:user"
)

// PATIntrospection is the detailed result of token introspection.
type PATIntrospection struct {
	Active    bool       `json:"active"`
	TokenID   uuid.UUID  `json:"token_id,omitempty"`
	UserID    uuid.UUID  `json:"user_id,omitempty"`
	Username  string     `json:"username,omitempty"`
	Name      string     `json:"name,omitempty"`
	Scopes    []string   `json:"scopes,omitempty"`
	CreatedAt time.Time  `json:"created_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type PATInfo struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Options struct {
	AllowPublicRegistration bool
	MaxLoginAttempts        int
	LockoutDuration         time.Duration
}

type Service struct {
	pool                    *pgxpool.Pool
	jwtSecret               []byte
	allowPublicRegistration bool
	maxLoginAttempts        int
	lockoutDuration         time.Duration
}

func NewService(pool *pgxpool.Pool, jwtSecret string, opts ...Options) *Service {
	o := Options{
		MaxLoginAttempts: 5,
		LockoutDuration:  15 * time.Minute,
	}
	if len(opts) > 0 {
		if opts[0].MaxLoginAttempts > 0 {
			o.MaxLoginAttempts = opts[0].MaxLoginAttempts
		}
		if opts[0].LockoutDuration > 0 {
			o.LockoutDuration = opts[0].LockoutDuration
		}
		o.AllowPublicRegistration = opts[0].AllowPublicRegistration
	}
	return &Service{
		pool:                    pool,
		jwtSecret:               []byte(jwtSecret),
		allowPublicRegistration: o.AllowPublicRegistration,
		maxLoginAttempts:        o.MaxLoginAttempts,
		lockoutDuration:         o.LockoutDuration,
	}
}

func (s *Service) AllowPublicRegistration() bool {
	return s.allowPublicRegistration
}

func (s *Service) Register(ctx context.Context, username, email, password string) (*User, error) {
	return s.createUser(ctx, username, email, password, RoleUser)
}

func (s *Service) CreateUser(ctx context.Context, username, email, password, role string) (*User, error) {
	if role != RoleAdmin && role != RoleUser {
		role = RoleUser
	}
	return s.createUser(ctx, username, email, password, role)
}

func (s *Service) createUser(ctx context.Context, username, email, password, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, email, role, COALESCE(avatar_url,''), created_at`,
		username, email, string(hash), role,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &u, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, *User, error) {
	// Check if account is locked
	var lockedUntil *time.Time
	err := s.pool.QueryRow(ctx, `SELECT locked_until FROM users WHERE username=$1 OR email=$1`, username).Scan(&lockedUntil)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, err
	}
	if lockedUntil != nil && time.Now().Before(*lockedUntil) {
		return "", nil, ErrAccountLocked
	}

	var u User
	var hash string
	err = s.pool.QueryRow(ctx, `
		SELECT id, username, email, role, COALESCE(avatar_url,''), password_hash, created_at
		FROM users WHERE username=$1 OR email=$1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &hash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		// Increment failed attempts and potentially lock account
		_, _ = s.pool.Exec(ctx, `
			UPDATE users SET failed_login_attempts = failed_login_attempts + 1
			WHERE id=$1`, u.ID)

		// Check if we should lock the account
		var attempts int
		_ = s.pool.QueryRow(ctx, `SELECT failed_login_attempts FROM users WHERE id=$1`, u.ID).Scan(&attempts)
		if attempts >= s.maxLoginAttempts {
			lockUntil := time.Now().Add(s.lockoutDuration)
			_, _ = s.pool.Exec(ctx, `UPDATE users SET locked_until=$1, failed_login_attempts=0 WHERE id=$2`, lockUntil, u.ID)
		}
		return "", nil, ErrInvalidCredentials
	}

	// Reset failed attempts on successful login
	_, _ = s.pool.Exec(ctx, `UPDATE users SET failed_login_attempts=0, locked_until=NULL WHERE id=$1`, u.ID)

	token, err := s.issueJWT(u.ID, u.Username)
	if err != nil {
		return "", nil, err
	}
	return token, &u, nil
}

func (s *Service) issueJWT(userID uuid.UUID, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID.String(),
		"username": username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *Service) ValidateToken(tokenStr string) (uuid.UUID, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil, "", ErrUnauthorized
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, "", ErrUnauthorized
	}
	sub, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, "", ErrUnauthorized
	}
	return id, username, nil
}

func (s *Service) CreatePAT(ctx context.Context, userID uuid.UUID, name string, scopes []string) (string, error) {
	raw, err := randomToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(raw)
	expiresAt := time.Now().Add(90 * 24 * time.Hour)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO personal_access_tokens (user_id, name, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, name, hash, scopes, expiresAt)
	if err != nil {
		return "", err
	}
	return "ghp_" + raw, nil
}

func (s *Service) ValidatePAT(ctx context.Context, token string) (uuid.UUID, error) {
	userID, _, err := s.ValidatePATWithScopes(ctx, token)
	return userID, err
}

func (s *Service) ValidatePATWithScopes(ctx context.Context, token string) (uuid.UUID, []string, error) {
	token = strings.TrimPrefix(token, "ghp_")
	hash := hashToken(token)
	var userID uuid.UUID
	var scopes []string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, scopes FROM personal_access_tokens
		WHERE token_hash=$1 AND (expires_at IS NULL OR expires_at > NOW())`, hash,
	).Scan(&userID, &scopes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil, ErrUnauthorized
		}
		return uuid.Nil, nil, err
	}
	return userID, scopes, nil
}

// IntrospectPAT validates a PAT and returns detailed token information
// suitable for client-side validation. Returns inactive=false for invalid/expired tokens.
func (s *Service) IntrospectPAT(ctx context.Context, token string) (*PATIntrospection, error) {
	token = strings.TrimPrefix(token, "ghp_")
	hash := hashToken(token)

	var info PATIntrospection
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT pat.id, pat.user_id, pat.name, pat.scopes, pat.created_at, pat.expires_at,
		       u.username
		FROM personal_access_tokens pat
		JOIN users u ON u.id = pat.user_id
		WHERE pat.token_hash=$1`, hash,
	).Scan(&info.TokenID, &userID, &info.Name, &info.Scopes, &info.CreatedAt, &info.ExpiresAt, &info.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PATIntrospection{Active: false}, nil
		}
		return nil, err
	}

	info.UserID = userID
	// Check if token is expired
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return &PATIntrospection{Active: false, TokenID: info.TokenID, UserID: userID,
			Username: info.Username, Name: info.Name, Scopes: info.Scopes,
			CreatedAt: info.CreatedAt, ExpiresAt: info.ExpiresAt}, nil
	}

	info.Active = true
	return &info, nil
}

func HasScope(scopes []string, required string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, s := range scopes {
		if s == required {
			return true
		}
		if required == ScopeRepo && (s == ScopeRepoWrite || s == ScopeWorkflow) {
			return true
		}
		if required == ScopeRepoWrite && s == ScopeRepoWrite {
			return true
		}
	}
	return false
}

func (s *Service) ListPATs(ctx context.Context, userID uuid.UUID) ([]PATInfo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, scopes, expires_at, created_at FROM personal_access_tokens
		WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PATInfo
	for rows.Next() {
		var p PATInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.Scopes, &p.ExpiresAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) RevokePAT(ctx context.Context, userID, patID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM personal_access_tokens WHERE id=$1 AND user_id=$2`, patID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// ListExpiringPATs returns tokens that expire within the given duration from now.
// It groups them by user_id so callers can send one notification per user.
func (s *Service) ListExpiringPATs(ctx context.Context, within time.Duration) (map[uuid.UUID][]PATInfo, error) {
	cutoff := time.Now().Add(within)
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, scopes, expires_at, created_at
		FROM personal_access_tokens
		WHERE expires_at IS NOT NULL AND expires_at <= $1 AND expires_at > NOW()
		ORDER BY user_id, expires_at`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]PATInfo)
	for rows.Next() {
		var userID uuid.UUID
		var p PATInfo
		if err := rows.Scan(&p.ID, &userID, &p.Name, &p.Scopes, &p.ExpiresAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], p)
	}
	return result, rows.Err()
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, email, role, COALESCE(avatar_url,''), created_at
		FROM users WHERE username=$1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	return &u, nil
}

func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, email, role, COALESCE(avatar_url,''), created_at
		FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	var role string
	err := s.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrUnauthorized
		}
		return false, err
	}
	return role == RoleAdmin, nil
}

func (s *Service) BootstrapAdmin(ctx context.Context, username, email, password string) error {
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.createUser(ctx, username, email, password, RoleAdmin)
	return err
}

func (s *Service) SearchUsers(ctx context.Context, prefix string, limit int) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, email, role, COALESCE(avatar_url,''), created_at
		FROM users WHERE username ILIKE $1 || '%' ORDER BY username ASC LIMIT $2`, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, email, role, COALESCE(avatar_url,''), created_at
		FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) DeleteUser(ctx context.Context, actorID, targetID uuid.UUID) error {
	if actorID == targetID {
		return ErrForbidden
	}
	var targetRole string
	if err := s.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, targetID).Scan(&targetRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		return err
	}
	if targetRole == RoleAdmin {
		var adminCount int
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role=$1`, RoleAdmin).Scan(&adminCount); err != nil {
			return err
		}
		if adminCount <= 1 {
			return ErrForbidden
		}
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, targetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// UpdateUserRole changes the role of a user. Only admins can change roles.
// Prevents removing the last admin.
func (s *Service) UpdateUserRole(ctx context.Context, actorID, targetID uuid.UUID, newRole string) (*User, error) {
	if newRole != RoleAdmin && newRole != RoleUser {
		return nil, fmt.Errorf("invalid role: %q", newRole)
	}
	var targetRole string
	if err := s.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, targetID).Scan(&targetRole); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	// Prevent removing the last admin
	if targetRole == RoleAdmin && newRole != RoleAdmin {
		var adminCount int
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role=$1`, RoleAdmin).Scan(&adminCount); err != nil {
			return nil, err
		}
		if adminCount <= 1 {
			return nil, ErrForbidden
		}
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET role=$1 WHERE id=$2
		RETURNING id, username, email, role, COALESCE(avatar_url,''), created_at`,
		newRole, targetID,
	).Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	return &u, nil
}

// IsLocked returns true if the account is currently locked due to failed login attempts.
func (s *Service) IsLocked(ctx context.Context, userID uuid.UUID) (bool, error) {
	var lockedUntil *time.Time
	err := s.pool.QueryRow(ctx, `SELECT locked_until FROM users WHERE id=$1`, userID).Scan(&lockedUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrUnauthorized
		}
		return false, err
	}
	return lockedUntil != nil && time.Now().Before(*lockedUntil), nil
}

// UnlockUser clears the lockout for a user. Only admins should call this.
func (s *Service) UnlockUser(ctx context.Context, userID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET failed_login_attempts=0, locked_until=NULL WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}
