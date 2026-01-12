package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// mockWSServer creates a mock WebSocket server
func mockWSServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade: %v", err)
			return
		}
		defer conn.Close()

		if handler != nil {
			handler(conn)
		}
	}))

	return server
}

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "Disconnected"},
		{StateConnecting, "Connecting"},
		{StateConnected, "Connected"},
		{StateReady, "Ready"},
		{ConnectionState(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("ConnectionState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ReconnectInterval != 5*time.Second {
		t.Errorf("ReconnectInterval = %v, want %v", cfg.ReconnectInterval, 5*time.Second)
	}
	if cfg.MaxReconnectAttempts != 0 {
		t.Errorf("MaxReconnectAttempts = %v, want %v", cfg.MaxReconnectAttempts, 0)
	}
	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval = %v, want %v", cfg.HeartbeatInterval, 30*time.Second)
	}
	if cfg.ReadTimeout != 90*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 90*time.Second)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 10*time.Second)
	}
}

func TestNewClient(t *testing.T) {
	// Test using default configuration
	client := NewClient(nil, nil)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.GetState() != StateDisconnected {
		t.Errorf("Initial state = %v, want %v", client.GetState(), StateDisconnected)
	}

	// Test using custom configuration
	cfg := &Config{
		ServerURL:         "ws://localhost:8080/ws",
		APIToken:          "test-token",
		ReconnectInterval: 10 * time.Second,
		HeartbeatInterval: 60 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      20 * time.Second,
	}
	client2 := NewClient(cfg, nil)
	if client2 == nil {
		t.Fatal("NewClient with config returned nil")
	}
}

func TestClient_Connect(t *testing.T) {
	// Create mock server
	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Send ConnectionAck
		ack := &mmv1.Message{
			Type:      mmv1.MessageType_MESSAGE_TYPE_CONNECTION_ACK,
			Timestamp: time.Now().UnixMilli(),
			Payload: &mmv1.Message_ConnectionAck{
				ConnectionAck: &mmv1.ConnectionAck{
					Success:   true,
					SessionId: "test-session",
					MmId:      "0x1234",
				},
			},
		}
		data, _ := proto.Marshal(ack)
		conn.WriteMessage(websocket.BinaryMessage, data)

		// Keep connection open
		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	// Convert URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	cfg := &Config{
		ServerURL:         wsURL,
		APIToken:          "test-token",
		ReconnectInterval: 1 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	client := NewClient(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify connection state
	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	// Cleanup
	client.Close()
}

func TestClient_Send(t *testing.T) {
	receivedCh := make(chan *mmv1.Message, 1)

	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Send ConnectionAck
		ack := &mmv1.Message{
			Type:      mmv1.MessageType_MESSAGE_TYPE_CONNECTION_ACK,
			Timestamp: time.Now().UnixMilli(),
			Payload: &mmv1.Message_ConnectionAck{
				ConnectionAck: &mmv1.ConnectionAck{
					Success:   true,
					SessionId: "test-session",
				},
			},
		}
		data, _ := proto.Marshal(ack)
		conn.WriteMessage(websocket.BinaryMessage, data)

		// Read message sent by client
		_, msgData, err := conn.ReadMessage()
		if err != nil {
			return
		}

		msg := &mmv1.Message{}
		if err := proto.Unmarshal(msgData, msg); err != nil {
			return
		}
		receivedCh <- msg
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := &Config{
		ServerURL:         wsURL,
		ReconnectInterval: 1 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	client := NewClient(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// Send message
	msg := &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_Heartbeat{
			Heartbeat: &mmv1.Heartbeat{Ping: true},
		},
	}

	if err := client.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify server received message
	select {
	case received := <-receivedCh:
		if received.Type != mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT {
			t.Errorf("Received type = %v, want %v", received.Type, mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestClient_SetMessageHandler(t *testing.T) {
	handlerCalled := make(chan bool, 1)

	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Send ConnectionAck
		ack := &mmv1.Message{
			Type:      mmv1.MessageType_MESSAGE_TYPE_CONNECTION_ACK,
			Timestamp: time.Now().UnixMilli(),
			Payload: &mmv1.Message_ConnectionAck{
				ConnectionAck: &mmv1.ConnectionAck{Success: true},
			},
		}
		data, _ := proto.Marshal(ack)
		conn.WriteMessage(websocket.BinaryMessage, data)

		// Send heartbeat
		hb := &mmv1.Message{
			Type:      mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT,
			Timestamp: time.Now().UnixMilli(),
			Payload: &mmv1.Message_Heartbeat{
				Heartbeat: &mmv1.Heartbeat{Ping: true},
			},
		}
		data, _ = proto.Marshal(hb)
		conn.WriteMessage(websocket.BinaryMessage, data)

		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cfg := &Config{
		ServerURL:         wsURL,
		ReconnectInterval: 1 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	client := NewClient(cfg, nil)

	// Set message handler
	client.SetMessageHandler(func(msg *mmv1.Message) error {
		if msg.Type == mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT {
			handlerCalled <- true
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// Wait for handler to be called
	select {
	case <-handlerCalled:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Message handler was not called")
	}
}

func TestClient_SendWhenNotConnected(t *testing.T) {
	cfg := &Config{
		ServerURL: "ws://localhost:9999/ws",
	}
	client := NewClient(cfg, nil)

	msg := &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT,
		Timestamp: time.Now().UnixMilli(),
	}

	err := client.Send(msg)
	if err == nil {
		t.Error("Send should fail when not connected")
	}
}
