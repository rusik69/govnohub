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
	ErrUserExists         = errors.New("user already exists")
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
}

func NewService(pool *pgxpool.Pool, jwtSecret string) *Service {
	return &Service{pool: pool, jwtSecret: []byte(jwtSecret)}
}

func (s *Service) Register(ctx context.Context, username, email, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, username, email, COALESCE(avatar_url,''), created_at`,
		username, email, string(hash),
	).Scan(&u.ID, &u.Username, &u.Email, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &u, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, *User, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, email, COALESCE(avatar_url,''), password_hash, created_at
		FROM users WHERE username=$1 OR email=$1`, username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.AvatarURL, &hash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}
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
	_, err = s.pool.Exec(ctx, `
		INSERT INTO personal_access_tokens (user_id, name, token_hash, scopes)
		VALUES ($1, $2, $3, $4)`, userID, name, hash, scopes)
	if err != nil {
		return "", err
	}
	return "ghp_" + raw, nil
}

func (s *Service) ValidatePAT(ctx context.Context, token string) (uuid.UUID, error) {
	token = strings.TrimPrefix(token, "ghp_")
	hash := hashToken(token)
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM personal_access_tokens
		WHERE token_hash=$1 AND (expires_at IS NULL OR expires_at > NOW())`, hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrUnauthorized
		}
		return uuid.Nil, err
	}
	return userID, nil
}

func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, email, COALESCE(avatar_url,''), created_at
		FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.AvatarURL, &u.CreatedAt)
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
