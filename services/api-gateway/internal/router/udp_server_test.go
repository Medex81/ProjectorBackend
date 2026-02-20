// services/api-gateway/internal/router/udp_server_test.go
package router

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUDPServer_BasicCommunication(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			UDPPort: 0, // Let OS assign port
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	// Create UDP server
	udpServer := NewUDPServer(cfg, jwtManager)
	err = udpServer.Start()
	require.NoError(t, err)
	defer udpServer.Stop()

	// Get actual port
	_, port, err := net.SplitHostPort(udpServer.Addr().String())
	require.NoError(t, err)

	// Connect client
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: parseInt(port),
	})
	require.NoError(t, err)
	defer conn.Close()

	// Create ENET-like packet
	packet := createENETPacket(token, []byte("ping"))

	// Send packet
	_, err = conn.Write(packet)
	require.NoError(t, err)

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Read response
	response := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(response)
	require.NoError(t, err)

	// Verify response
	assert.True(t, bytes.HasPrefix(response[:n], []byte("pong")))
}

func TestUDPServer_Authentication(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			UDPPort: 0,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	validToken, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	expiredManager := auth.NewJWTManager(cfg.Auth.JWTSecret, -1*time.Hour)
	expiredToken, err := expiredManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	udpServer := NewUDPServer(cfg, jwtManager)
	err = udpServer.Start()
	require.NoError(t, err)
	defer udpServer.Stop()

	_, port, err := net.SplitHostPort(udpServer.Addr().String())
	require.NoError(t, err)

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: parseInt(port),
	})
	require.NoError(t, err)
	defer conn.Close()

	tests := []struct {
		name          string
		token         string
		expectSuccess bool
	}{
		{
			name:          "Valid token",
			token:         validToken,
			expectSuccess: true,
		},
		{
			name:          "No token",
			token:         "",
			expectSuccess: false,
		},
		{
			name:          "Invalid token",
			token:         "invalid-token",
			expectSuccess: false,
		},
		{
			name:          "Expired token",
			token:         expiredToken,
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := createENETPacket(tt.token, []byte("ping"))

			conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
			_, err := conn.Write(packet)
			require.NoError(t, err)

			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			response := make([]byte, 1024)
			n, _, err := conn.ReadFromUDP(response)

			if tt.expectSuccess {
				assert.NoError(t, err)
				assert.True(t, bytes.HasPrefix(response[:n], []byte("pong")))
			} else {
				assert.Error(t, err)
				assert.True(t, err.(net.Error).Timeout())
			}
		})
	}
}

func TestUDPServer_PacketFragmentation(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			UDPPort: 0,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	udpServer := NewUDPServer(cfg, jwtManager)
	err = udpServer.Start()
	require.NoError(t, err)
	defer udpServer.Stop()

	_, port, err := net.SplitHostPort(udpServer.Addr().String())
	require.NoError(t, err)

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: parseInt(port),
	})
	require.NoError(t, err)
	defer conn.Close()

	// Test with different packet sizes
	sizes := []int{64, 256, 512, 1024, 2048, 4096}

	for _, size := range sizes {
		t.Run("Size_"+string(rune(size)), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			packet := createENETPacket(token, data)

			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, err := conn.Write(packet)
			require.NoError(t, err)

			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			response := make([]byte, size+1024) // Extra space for protocol overhead
			n, _, err := conn.ReadFromUDP(response)

			assert.NoError(t, err)
			assert.Greater(t, n, 0)
		})
	}
}

func TestUDPServer_HighLoad(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			UDPPort: 0,
		},
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	udpServer := NewUDPServer(cfg, jwtManager)
	err = udpServer.Start()
	require.NoError(t, err)
	defer udpServer.Stop()

	_, port, err := net.SplitHostPort(udpServer.Addr().String())
	require.NoError(t, err)

	// Create multiple clients
	clientCount := 10
	packetsPerClient := 100

	results := make(chan bool, clientCount*packetsPerClient)

	for i := 0; i < clientCount; i++ {
		go func(clientID int) {
			conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: parseInt(port),
			})
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()

			for j := 0; j < packetsPerClient; j++ {
				packet := createENETPacket(token, []byte("ping"))

				conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
				_, err := conn.Write(packet)
				if err != nil {
					results <- false
					continue
				}

				conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				response := make([]byte, 1024)
				n, _, err := conn.ReadFromUDP(response)

				if err == nil && bytes.HasPrefix(response[:n], []byte("pong")) {
					results <- true
				} else {
					results <- false
				}
			}
		}(i)
	}

	// Collect results
	successCount := 0
	totalCount := clientCount * packetsPerClient

	for i := 0; i < totalCount; i++ {
		if <-results {
			successCount++
		}
	}

	// Should have high success rate (>95%)
	successRate := float64(successCount) / float64(totalCount) * 100
	assert.Greater(t, successRate, 95.0, "Success rate should be >95%")
}

// Helper functions
func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

func createENETPacket(token string, data []byte) []byte {
	// Format: [token length (4 bytes)][token][data]
	tokenBytes := []byte(token)
	packet := make([]byte, 4+len(tokenBytes)+len(data))

	binary.BigEndian.PutUint32(packet[:4], uint32(len(tokenBytes)))
	copy(packet[4:], tokenBytes)
	copy(packet[4+len(tokenBytes):], data)

	return packet
}
