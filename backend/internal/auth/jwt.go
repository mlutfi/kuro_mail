package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/webmail/backend/internal/config"
	"github.com/webmail/backend/internal/models"
)

type JWTService struct {
	cfg *config.Config
}

func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{cfg: cfg}
}

type accessClaims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	Type      string `json:"type"`
	jwt.RegisteredClaims
}

type refreshClaims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	Type      string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateAccessToken membuat JWT access token (short-lived: 1 jam)
func (j *JWTService) GenerateAccessToken(userID, sessionID uuid.UUID, jti string) (string, error) {
	claims := accessClaims{
		UserID:    userID.String(),
		SessionID: sessionID.String(),
		Type:      "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.cfg.JWT.AccessExpiry)),
			Issuer:    "webmail",
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.cfg.JWT.AccessSecret))
}

// GenerateRefreshToken membuat JWT refresh token (long-lived: 7 hari)
func (j *JWTService) GenerateRefreshToken(userID, sessionID uuid.UUID, jti string) (string, error) {
	claims := refreshClaims{
		UserID:    userID.String(),
		SessionID: sessionID.String(),
		Type:      "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.cfg.JWT.RefreshExpiry)),
			Issuer:    "webmail",
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.cfg.JWT.RefreshSecret))
}

// ValidateAccessToken memvalidasi access token dan return claims
func (j *JWTService) ValidateAccessToken(tokenStr string) (*models.JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &accessClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(j.cfg.JWT.AccessSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*accessClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if claims.Type != "access" {
		return nil, errors.New("token type mismatch")
	}

	return &models.JWTClaims{
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
		JTI:       claims.ID,
		Type:      claims.Type,
	}, nil
}

// ValidateRefreshToken memvalidasi refresh token
func (j *JWTService) ValidateRefreshToken(tokenStr string) (*models.JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &refreshClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(j.cfg.JWT.RefreshSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*refreshClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if claims.Type != "refresh" {
		return nil, errors.New("token type mismatch")
	}

	return &models.JWTClaims{
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
		JTI:       claims.ID,
		Type:      claims.Type,
	}, nil
}
