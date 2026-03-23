package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"test-backend/internal/auth"
	"test-backend/internal/model"
)

type jwtContextKey string

const principalKey jwtContextKey = "principal"

type principal struct {
	UserID uuid.UUID
	Role   model.Role
}

func JWTMiddleware(secret []byte, log *logrus.Entry) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/dummyLogin" || path == "/register" || path == "/login" || path == "/_info" ||
				strings.HasPrefix(path, "/swagger/") || strings.HasPrefix(path, "/docs/") {
				next.ServeHTTP(w, r)
				return
			}
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer ") {
				log.WithField("path", path).Warn("missing bearer token")
				writeError(w, http.StatusUnauthorized, INVALIDREQUEST, "invalid request")
				return
			}
			raw := strings.TrimPrefix(authz, "Bearer ")
			userID, role, err := auth.ParseToken(secret, raw)
			if err != nil {
				log.WithField("path", path).Warn("invalid jwt")
				writeError(w, http.StatusUnauthorized, INVALIDREQUEST, "invalid request")
				return
			}
			ctx := context.WithValue(r.Context(), principalKey, principal{UserID: userID, Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
