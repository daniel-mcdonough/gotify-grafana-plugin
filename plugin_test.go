package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotify/plugin-api"
	"github.com/stretchr/testify/assert"
)

func TestGetGotifyPluginInfo(t *testing.T) {
	info := GetGotifyPluginInfo()
	assert.NotEmpty(t, info.Author)
	assert.NotEmpty(t, info.Description)
	assert.NotEmpty(t, info.License)
	assert.NotEmpty(t, info.ModulePath)
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.Website)
}

func TestAPICompatibility(t *testing.T) {
	p := &WebhookForwarderPlugin{}
	assert.Implements(t, (*plugin.Plugin)(nil), p)
	assert.Implements(t, (*plugin.Webhooker)(nil), p)
	assert.Implements(t, (*plugin.Messenger)(nil), p)
	assert.Implements(t, (*plugin.Displayer)(nil), p)
}

func TestWebhookForwarderPlugin_Enable(t *testing.T) {
	p := &WebhookForwarderPlugin{}
	err := p.Enable()
	assert.NoError(t, err)
}

func TestWebhookForwarderPlugin_Disable(t *testing.T) {
	p := &WebhookForwarderPlugin{}
	err := p.Disable()
	assert.NoError(t, err)
}

// MockMessageHandler implements plugin.MessageHandler for testing
type MockMessageHandler struct {
	sentMessages []plugin.Message
	shouldFail   bool
}

func (m *MockMessageHandler) SendMessage(msg plugin.Message) error {
	if m.shouldFail {
		return assert.AnError
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func TestWebhookForwarderPlugin_HandleWebhookMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	tests := []struct {
		name           string
		payload        interface{}
		expectedStatus int
		expectedMsg    *plugin.Message
		handlerFails   bool
	}{
		{
			name: "valid message with all fields",
			payload: map[string]interface{}{
				"title":    "Test Title",
				"message":  "Test Message",
				"priority": 7,
				"extras":   map[string]interface{}{"key": "value"},
			},
			expectedStatus: http.StatusOK,
			expectedMsg: &plugin.Message{
				Title:    "Test Title",
				Message:  "Test Message",
				Priority: 7,
				Extras:   map[string]interface{}{"key": "value"},
			},
		},
		{
			name: "message without title uses default",
			payload: map[string]interface{}{
				"message": "Test Message",
			},
			expectedStatus: http.StatusOK,
			expectedMsg: &plugin.Message{
				Title:    "Webhook Message",
				Message:  "Test Message",
				Priority: 5,
			},
		},
		{
			name: "invalid priority gets default",
			payload: map[string]interface{}{
				"message":  "Test Message",
				"priority": 15,
			},
			expectedStatus: http.StatusOK,
			expectedMsg: &plugin.Message{
				Title:    "Webhook Message",
				Message:  "Test Message",
				Priority: 5,
			},
		},
		{
			name:           "missing message field",
			payload:        map[string]interface{}{"title": "Title Only"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			payload:        "not json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "handler fails",
			payload: map[string]interface{}{
				"message": "Test Message",
			},
			expectedStatus: http.StatusInternalServerError,
			handlerFails:   true,
		},
		{
			name: "grafana firing alert",
			payload: map[string]interface{}{
				"title":   "[FIRING:1] TestAlert Grafana",
				"status":  "firing",
				"state":   "alerting",
				"message": "**Firing**\n\nValue: [no value]\nLabels:\n - alertname = TestAlert",
				"alerts":  []interface{}{},
			},
			expectedStatus: http.StatusOK,
			expectedMsg: &plugin.Message{
				Title:    "[FIRING:1] TestAlert Grafana",
				Message:  "**Firing**\n\nValue: [no value]\nLabels:\n - alertname = TestAlert",
				Priority: 8,
				Extras: map[string]interface{}{
					"source": "grafana",
					"status": "firing",
					"state":  "alerting",
				},
			},
		},
		{
			name: "grafana resolved alert",
			payload: map[string]interface{}{
				"title":   "[RESOLVED] TestAlert",
				"status":  "resolved",
				"state":   "ok",
				"message": "Alert resolved",
				"alerts":  []interface{}{},
			},
			expectedStatus: http.StatusOK,
			expectedMsg: &plugin.Message{
				Title:    "[RESOLVED] TestAlert",
				Message:  "Alert resolved",
				Priority: 3,
				Extras: map[string]interface{}{
					"source": "grafana",
					"status": "resolved",
					"state":  "ok",
				},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := &MockMessageHandler{shouldFail: tt.handlerFails}
			p := &WebhookForwarderPlugin{
				msgHandler: mockHandler,
				userCtx:    plugin.UserContext{Name: "testuser"},
			}
			
			router := gin.New()
			router.POST("/message", p.handleWebhookMessage)
			
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/message", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			
			router.ServeHTTP(w, req)
			
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			if tt.expectedMsg != nil && !tt.handlerFails {
				assert.Len(t, mockHandler.sentMessages, 1)
				assert.Equal(t, *tt.expectedMsg, mockHandler.sentMessages[0])
			}
		})
	}
}

func TestWebhookForwarderPlugin_HandleInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	p := &WebhookForwarderPlugin{
		userCtx: plugin.UserContext{Name: "testuser"},
	}
	
	router := gin.New()
	router.GET("/", p.handleInfo)
	
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Webhook Forwarder", response["plugin"])
	assert.Equal(t, "testuser", response["user"])
	assert.Contains(t, response, "endpoints")
}

func TestWebhookForwarderPlugin_GetDisplay(t *testing.T) {
	p := &WebhookForwarderPlugin{
		userCtx: plugin.UserContext{Name: "testuser"},
	}
	
	// Test with nil location
	display := p.GetDisplay(nil)
	assert.Contains(t, display, "Webhook Forwarder Plugin")
	assert.Contains(t, display, "testuser")
	assert.Contains(t, display, "POST")
	assert.Contains(t, display, "Grafana")
	
	// Test with valid location
	location, _ := url.Parse("https://gotify.example.com/plugin/5/display")
	display = p.GetDisplay(location)
	assert.Contains(t, display, "https://gotify.example.com/plugin/5/message")
	assert.Contains(t, display, "testuser")
}