package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInternalServer  = errors.New("internal server error")
)

type AuthService struct {
	store     *repository.Store
	userCache *userCache
}

func NewAuthService(store *repository.Store) *AuthService {
	cacheTTL := parseDurationEnv("AUTH_USER_CACHE_TTL", 5*time.Second)
	cacheSize := parseIntEnv("AUTH_USER_CACHE_SIZE", 1024)
	var cache *userCache
	if cacheTTL > 0 && cacheSize > 0 {
		cache = newUserCache(cacheTTL, cacheSize)
	}
	return &AuthService{store: store, userCache: cache}
}

func (s *AuthService) Login(ctx context.Context, userName, password string) (string, time.Time, error) {
	ctx, span := otel.Tracer("service.auth").Start(ctx, "AuthService.Login")
	defer span.End()

	var sessionID string
	var expiresAt time.Time
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		fetchStart := time.Now()
		user, err := s.getUser(ctx, userName)
		if err != nil {
			log.Printf("[Login] ユーザー検索失敗(userName: %s): %v", userName, err)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return ErrInternalServer
		}
		span.AddEvent("user fetched", traceAttributesFromDuration("user_lookup_ms", fetchStart))

		verifyStart := time.Now()
		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
		if err != nil {
			log.Printf("[Login] パスワード検証失敗: %v", err)
			span.RecordError(err)
			return ErrInvalidPassword
		}
		span.AddEvent("password verified", traceAttributesFromDuration("password_verify_ms", verifyStart))

		sessionStart := time.Now()
		sessionDuration := 24 * time.Hour
		sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		if err != nil {
			log.Printf("[Login] セッション生成失敗: %v", err)
			return ErrInternalServer
		}
		span.AddEvent("session created", traceAttributesFromDuration("session_create_ms", sessionStart))
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	log.Printf("Login successful for UserName '%s', session created.", userName)
	return sessionID, expiresAt, nil
}

func (s *AuthService) getUser(ctx context.Context, userName string) (*model.User, error) {
	if s.userCache != nil {
		if cached := s.userCache.get(userName); cached != nil {
			return cached, nil
		}
	}
	user, err := s.store.UserRepo.FindByUserName(ctx, userName)
	if err != nil {
		return nil, err
	}
	if s.userCache != nil {
		s.userCache.set(userName, user)
	}
	return user, nil
}

func traceAttributesFromDuration(key string, start time.Time) trace.EventOption {
	dur := time.Since(start).Milliseconds()
	return trace.WithAttributes(attribute.Int64(key, dur))
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return fallback
}

func parseIntEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return fallback
}

type userCache struct {
	mx         sync.RWMutex
	entries    map[string]cachedUser
	ttl        time.Duration
	maxEntries int
}

type cachedUser struct {
	user      model.User
	expiresAt time.Time
}

func newUserCache(ttl time.Duration, maxEntries int) *userCache {
	return &userCache{
		entries:    make(map[string]cachedUser, maxEntries),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (c *userCache) get(userName string) *model.User {
	c.mx.RLock()
	entry, ok := c.entries[userName]
	c.mx.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			c.mx.Lock()
			delete(c.entries, userName)
			c.mx.Unlock()
		}
		return nil
	}
	userCopy := entry.user
	return &userCopy
}

func (c *userCache) set(userName string, user *model.User) {
	if user == nil {
		return
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	if len(c.entries) >= c.maxEntries {
		c.evictExpiredLocked()
		if len(c.entries) >= c.maxEntries {
			c.evictOldestLocked()
		}
	}
	c.entries[userName] = cachedUser{
		user:      *user,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *userCache) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *userCache) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range c.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
