// services/api-gateway/test/mocks/mock_services.go
package mocks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
)

type MockAuthService struct {
	server     *httptest.Server
	mu         sync.RWMutex
	users      map[string]*MockUser
	codes      map[string]string
	jwtManager *auth.JWTManager
}

type MockUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	AppID     string    `json:"app_id"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

func NewMockAuthService() *MockAuthService {
	m := &MockAuthService{
		users:      make(map[string]*MockUser),
		codes:      make(map[string]string),
		jwtManager: auth.NewJWTManager("test-secret", 24*time.Hour),
	}

	m.server = httptest.NewServer(m)
	return m
}

func (m *MockAuthService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/auth/register":
		m.handleRegister(w, r)
	case "/auth/verify":
		m.handleVerify(w, r)
	case "/auth/login":
		m.handleLogin(w, r)
	case "/health":
		m.handleHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *MockAuthService) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		AppID    string `json:"app_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if user exists
	key := req.Email + ":" + req.AppID
	if user, exists := m.users[key]; exists && user.Verified {
		http.Error(w, "User already exists", http.StatusConflict)
		return
	}

	// Create or update user
	m.users[key] = &MockUser{
		ID:        "user-" + req.Email,
		Email:     req.Email,
		Password:  req.Password,
		AppID:     req.AppID,
		Verified:  false,
		CreatedAt: time.Now(),
	}

	// Generate verification code
	code := "123456"
	m.codes[key] = code

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Verification code sent",
	})
}

func (m *MockAuthService) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
		AppID string `json:"app_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := req.Email + ":" + req.AppID
	expectedCode, exists := m.codes[key]
	if !exists || expectedCode != req.Code {
		http.Error(w, "Invalid code", http.StatusUnauthorized)
		return
	}

	user, exists := m.users[key]
	if !exists {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	user.Verified = true
	delete(m.codes, key)

	// Generate token
	token, _ := m.jwtManager.Generate(user.ID, user.Email, user.AppID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_at": time.Now().Add(24 * time.Hour),
		"user_id":    user.ID,
		"email":      user.Email,
		"app_id":     user.AppID,
		"verified":   true,
	})
}

func (m *MockAuthService) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		AppID    string `json:"app_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := req.Email + ":" + req.AppID
	user, exists := m.users[key]
	if !exists || user.Password != req.Password || !user.Verified {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, _ := m.jwtManager.Generate(user.ID, user.Email, user.AppID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_at": time.Now().Add(24 * time.Hour),
		"user_id":    user.ID,
		"email":      user.Email,
		"app_id":     user.AppID,
	})
}

func (m *MockAuthService) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func (m *MockAuthService) URL() string {
	return m.server.URL
}

func (m *MockAuthService) Close() {
	m.server.Close()
}

func (m *MockAuthService) GetUser(email, appID string) *MockUser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.users[email+":"+appID]
}

// MockGameService
type MockGameService struct {
	server  *httptest.Server
	mu      sync.RWMutex
	games   map[string]*MockGame
	players map[string]*MockPlayer
}

type MockGame struct {
	ID      string                 `json:"id"`
	State   map[string]interface{} `json:"state"`
	Players []string               `json:"players"`
}

type MockPlayer struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Score    int    `json:"score"`
}

func NewMockGameService() *MockGameService {
	m := &MockGameService{
		games:   make(map[string]*MockGame),
		players: make(map[string]*MockPlayer),
	}

	m.server = httptest.NewServer(m)
	return m
}

func (m *MockGameService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify JWT token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.URL.Path {
	case "/game/state":
		m.handleGameState(w, r)
	case "/game/move":
		m.handleGameMove(w, r)
	case "/player/profile":
		m.handlePlayerProfile(w, r)
	case "/player/stats":
		m.handlePlayerStats(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *MockGameService) handleGameState(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("player_id")

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return mock game state
	json.NewEncoder(w).Encode(map[string]interface{}{
		"game_id": "game-123",
		"players": []string{playerID, "player-456"},
		"state": map[string]interface{}{
			"level": 1,
			"score": 100,
		},
		"status": "active",
	})
}

func (m *MockGameService) handleGameMove(w http.ResponseWriter, r *http.Request) {
	var move struct {
		GameID   string                 `json:"game_id"`
		PlayerID string                 `json:"player_id"`
		MoveData map[string]interface{} `json:"move_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&move); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted": true,
		"new_state": map[string]interface{}{
			"level": 2,
			"score": 200,
		},
		"message": "Move accepted",
	})
}

func (m *MockGameService) handlePlayerProfile(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("player_id")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       playerID,
		"username": "Player" + playerID,
		"level":    5,
		"elo":      1200,
		"status":   "online",
	})
}

func (m *MockGameService) handlePlayerStats(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"games_played":   50,
		"wins":           30,
		"losses":         15,
		"draws":          5,
		"current_streak": 3,
		"best_streak":    7,
		"total_score":    5000,
	})
}

func (m *MockGameService) URL() string {
	return m.server.URL
}

func (m *MockGameService) Close() {
	m.server.Close()
}
