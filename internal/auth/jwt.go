package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"test-backend/internal/apperrors"
	"test-backend/internal/model"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func ParseToken(secret []byte, token string) (uuid.UUID, model.Role, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, "", apperrors.Unauthorized
	}
	id, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, "", apperrors.Unauthorized
	}
	return id, model.Role(claims.Role), nil
}

func SignToken(secret []byte, userID uuid.UUID, role model.Role) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID.String(),
		Role:   string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
		},
	})
	return t.SignedString(secret)
}
