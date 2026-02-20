// services/api-gateway/internal/router/udp_server.go
package router

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/models"
)

const (
	maxPacketSize = 65507
	tokenSize     = 4 // bytes for token length
)

type UDPServer struct {
	config     *config.Config
	jwtManager *auth.JWTManager
	conn       *net.UDPConn
	clients    sync.Map
	stopChan   chan bool
	handlers   map[uint16]UDPHandler
}

type UDPHandler func(addr *net.UDPAddr, data []byte) ([]byte, error)

func NewUDPServer(cfg *config.Config, jwtManager *auth.JWTManager) *UDPServer {
	return &UDPServer{
		config:     cfg,
		jwtManager: jwtManager,
		stopChan:   make(chan bool),
		handlers:   make(map[uint16]UDPHandler),
	}
}

func (s *UDPServer) Start() error {
	addr := net.UDPAddr{
		Port: s.config.Gateway.UDPPort,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return fmt.Errorf("failed to start UDP server: %w", err)
	}

	s.conn = conn
	logger.Info().Int("port", s.config.Gateway.UDPPort).Msg("UDP server started")

	// Register default handlers
	s.registerDefaultHandlers()

	// Start cleanup routine
	go s.cleanupClients()

	// Start reading packets
	go s.readLoop()

	return nil
}

func (s *UDPServer) Stop() {
	close(s.stopChan)
	if s.conn != nil {
		s.conn.Close()
	}
	logger.Info().Msg("UDP server stopped")
}

func (s *UDPServer) Addr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *UDPServer) registerDefaultHandlers() {
	// Ping handler
	s.handlers[0x01] = func(addr *net.UDPAddr, data []byte) ([]byte, error) {
		return []byte("pong"), nil
	}

	// Authentication handler
	s.handlers[0x02] = s.handleAuthentication

	// Echo handler for testing
	s.handlers[0x03] = func(addr *net.UDPAddr, data []byte) ([]byte, error) {
		return data, nil
	}
}

func (s *UDPServer) readLoop() {
	buffer := make([]byte, maxPacketSize)

	for {
		select {
		case <-s.stopChan:
			return
		default:
			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				logger.Error().Err(err).Msg("Error reading UDP packet")
				continue
			}

			// Handle packet in goroutine
			go s.handlePacket(addr, buffer[:n])
		}
	}
}

func (s *UDPServer) handlePacket(addr *net.UDPAddr, data []byte) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error().Interface("recover", r).Msg("Panic in UDP handler")
		}
	}()

	// Parse packet
	if len(data) < 6 { // Minimum packet size: token length(4) + token + message type(2)
		logger.Warn().Int("size", len(data)).Msg("Packet too small")
		return
	}

	// Extract token
	tokenLen := binary.BigEndian.Uint32(data[:4])
	if int(tokenLen)+6 > len(data) {
		logger.Warn().Msg("Invalid token length")
		return
	}

	token := string(data[4 : 4+tokenLen])

	// Verify token
	claims, err := s.jwtManager.Verify(token)
	if err != nil {
		logger.Warn().Err(err).Msg("Invalid token in UDP packet")
		return
	}

	// Update client
	clientID := claims.UserID + ":" + claims.AppID
	client := &models.UDPClient{
		ID:          clientID,
		UserID:      claims.UserID,
		AppID:       claims.AppID,
		Address:     addr.String(),
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
	}
	s.clients.Store(addr.String(), client)
	metrics.ServiceActiveConnections.WithLabelValues("udp", "clients").Inc()

	// Extract message type and data
	messageType := binary.BigEndian.Uint16(data[4+tokenLen : 6+tokenLen])
	messageData := data[6+tokenLen:]

	// Find handler
	handler, exists := s.handlers[messageType]
	if !exists {
		logger.Warn().Uint16("type", messageType).Msg("Unknown message type")
		return
	}

	// Handle message
	response, err := handler(addr, messageData)
	if err != nil {
		logger.Error().Err(err).Msg("Error handling UDP message")
		return
	}

	// Send response
	if response != nil {
		s.sendResponse(addr, response)
	}

	// Update last seen
	client.LastSeen = time.Now()
	s.clients.Store(addr.String(), client)
}

func (s *UDPServer) sendResponse(addr *net.UDPAddr, data []byte) {
	_, err := s.conn.WriteToUDP(data, addr)
	if err != nil {
		logger.Error().Err(err).Msg("Error sending UDP response")
	}
}

func (s *UDPServer) handleAuthentication(addr *net.UDPAddr, data []byte) ([]byte, error) {
	// Authentication logic
	return []byte("authenticated"), nil
}

func (s *UDPServer) cleanupClients() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			now := time.Now()
			s.clients.Range(func(key, value interface{}) bool {
				client := value.(*models.UDPClient)
				if now.Sub(client.LastSeen) > 30*time.Minute {
					s.clients.Delete(key)
					metrics.ServiceActiveConnections.WithLabelValues("udp", "clients").Dec()
					logger.Debug().Str("client", client.ID).Msg("UDP client cleaned up")
				}
				return true
			})
		}
	}
}

func (s *UDPServer) GetClientCount() int {
	count := 0
	s.clients.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
