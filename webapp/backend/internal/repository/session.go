package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SessionRepository struct {
	db    DBTX
	cache *sessionCache
}

type sessionCache struct {
	mx         sync.RWMutex
	entries    map[string]cachedSession
	ttl        time.Duration
	maxEntries int
}

type cachedSession struct {
	userID    int
	expiresAt time.Time
}

func NewSessionRepository(db DBTX) *SessionRepository {
	cache := newSessionCache(300*time.Millisecond, 1000)
	return &SessionRepository{db: db, cache: cache}
}

func newSessionCache(ttl time.Duration, maxEntries int) *sessionCache {
	return &sessionCache{
		entries:    make(map[string]cachedSession, maxEntries),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// セッションを作成し、セッションIDと有効期限を返す
func (r *SessionRepository) Create(ctx context.Context, userBusinessID int, duration time.Duration) (string, time.Time, error) {
	sessionUUID, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(duration)
	sessionIDStr := sessionUUID.String()

	query := "INSERT INTO user_sessions (session_uuid, user_id, expires_at) VALUES (?, ?, ?)"
	_, err = r.db.ExecContext(ctx, query, sessionIDStr, userBusinessID, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return sessionIDStr, expiresAt, nil
}

// セッションIDからユーザーIDを取得
func (r *SessionRepository) FindUserBySessionID(ctx context.Context, sessionID string) (int, error) {
	// キャッシュから確認
	if cachedUserID := r.cache.get(sessionID); cachedUserID != 0 {
		return cachedUserID, nil
	}

	var userID int
	// JOINを避けて直接セッションテーブルから検索（パフォーマンス最適化）
	query := `
		SELECT 
			user_id
		FROM user_sessions 
		WHERE session_uuid = ? AND expires_at > ?`
	err := r.db.GetContext(ctx, &userID, query, sessionID, time.Now())
	if err != nil {
		return 0, err
	}

	// キャッシュに保存
	r.cache.set(sessionID, userID)
	
	return userID, nil
}

func (c *sessionCache) get(sessionID string) int {
	c.mx.RLock()
	entry, ok := c.entries[sessionID]
	c.mx.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			c.mx.Lock()
			delete(c.entries, sessionID)
			c.mx.Unlock()
		}
		return 0
	}
	return entry.userID
}

func (c *sessionCache) set(sessionID string, userID int) {
	if userID == 0 {
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
	c.entries[sessionID] = cachedSession{
		userID:    userID,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *sessionCache) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *sessionCache) evictOldestLocked() {
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
