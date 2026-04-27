package max

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"
)

type confirmAction struct {
	expiresAt time.Time
	execute   func() string
}

var (
	confirmMu      sync.Mutex
	pendingConfirm = map[string]confirmAction{}
)

func registerConfirm(chatID int64, ttl time.Duration, execute func() string) string {
	token := randomToken(8)
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := time.Now().UTC()
	confirmMu.Lock()
	defer confirmMu.Unlock()
	purgeExpiredLocked(now)
	pendingConfirm[confirmKey(chatID, token)] = confirmAction{
		expiresAt: now.Add(ttl),
		execute:   execute,
	}
	return token
}

func consumeConfirm(chatID int64, token string) (string, bool) {
	now := time.Now().UTC()
	confirmMu.Lock()
	key := confirmKey(chatID, token)
	action, ok := pendingConfirm[key]
	if !ok || now.After(action.expiresAt) {
		delete(pendingConfirm, key)
		confirmMu.Unlock()
		return "", false
	}
	delete(pendingConfirm, key)
	confirmMu.Unlock()
	return action.execute(), true
}

func purgeExpiredLocked(now time.Time) {
	for key, action := range pendingConfirm {
		if now.After(action.expiresAt) {
			delete(pendingConfirm, key)
		}
	}
}

func confirmKey(chatID int64, token string) string {
	return strconv.FormatInt(chatID, 10) + ":" + strings.TrimSpace(token)
}

func randomToken(size int) string {
	if size <= 0 {
		size = 8
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}
