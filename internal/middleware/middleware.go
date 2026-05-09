package middleware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/risbern21/api_gateway/internal/cache"
	"github.com/risbern21/api_gateway/internal/logger"
	"github.com/risbern21/api_gateway/internal/token"
)

type ResponseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

type Middleware struct {
	maker token.JWTMaker
	cache *redis.Client
}

func NewMiddleware(secretKey string) *Middleware {
	return &Middleware{
		maker: *token.NewJWTMaker(secretKey),
		cache: cache.Client(),
	}
}

func (rec *ResponseRecorder) Write(b []byte) (int, error) {
	rec.body.Write(b)
	return rec.ResponseWriter.Write(b)
}

func (rec *ResponseRecorder) WriteHeader(statusCode int) {
	rec.statusCode = statusCode
	rec.ResponseWriter.WriteHeader(statusCode)
}

func getIP(r *http.Request) (string, error) {
	ips := r.Header.Get("X-Forwarded-For")
	splitIPs := strings.Split(ips, ",")

	if len(splitIPs) > 0 {
		netIP := net.ParseIP(strings.TrimSpace(splitIPs[len(splitIPs)-1]))
		if netIP != nil {
			return netIP.String(), nil
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}

	netIP := net.ParseIP(ip)
	if netIP != nil {
		ip := netIP.String()
		if ip == "::1" {
			return "127.0.0.1", nil
		}
		return ip, nil
	}

	return "", errors.New("IP not found")
}

func (m *Middleware) RateLimitingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, err := getIP(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to fetch IP : %v", err), http.StatusBadRequest)
		}

		ctx := context.Background()
		count, _ := m.cache.Incr(ctx, ip).Result()
		if count == 1 {
			cache.Client().Expire(ctx, ip, 15*time.Second)
		}

		if count > 5 {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) AuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")

		if accessToken == "" || !strings.HasPrefix(accessToken, "Bearer ") {
			http.Error(w, "invalid access token", http.StatusUnauthorized)
			return
		}

		accessToken = strings.Split(accessToken, " ")[1]
		_, err := m.maker.VerifyToken(accessToken)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) CachingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := constructKey(r.Method, r.URL.Path)

		ctx := context.Background()
		content, err := m.cache.Get(ctx, key).Result()

		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(content))
			return
		}

		rec := &ResponseRecorder{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}
		next.ServeHTTP(rec, r)

		if rec.statusCode == http.StatusOK && rec.body.Len() > 0 {
			err := m.cache.Set(ctx, key, rec.body.String(), 60*time.Second).Err()
			if err != nil {
				logger.Log().Info("unable to cache the key")
			}
		}
	})
}

func constructKey(method string, path string) string {
	return fmt.Sprintf("%s-%s", method, path)
}

func (m *Middleware) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logger.Log().Infof("Received request : %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		logger.Log().Infof("Received response : %s %s in %s", r.Method, r.URL.Path, time.Since(start))
	})
}
