package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gotify/plugin-api"
)

// GetGotifyPluginInfo returns gotify plugin info.
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		ModulePath:  "github.com/gotify/webhook-forwarder",
		Version:     "1.0.0",
		Author:      "Gotify Community",
		Website:     "https://github.com/gotify/webhook-forwarder",
		Description: "Advanced webhook forwarder with native Grafana support. Receives webhook messages from external services and forwards them as Gotify notifications. Automatically detects and properly formats Grafana alerts with smart priority assignment.",
		License:     "MIT",
		Name:        "Webhook Forwarder",
	}
}

// WebhookMessage represents the expected webhook payload
type WebhookMessage struct {
	Title    string                 `json:"title"`
	Message  string                 `json:"message"`
	Priority int                    `json:"priority"`
	Extras   map[string]interface{} `json:"extras,omitempty"`
}

// GrafanaAlert represents a single alert in Grafana webhook
type GrafanaAlert struct {
	Status      string                 `json:"status"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]string      `json:"annotations"`
	StartsAt    string                 `json:"startsAt"`
	EndsAt      string                 `json:"endsAt"`
	SilenceURL  string                 `json:"silenceURL"`
	DashboardURL string                `json:"dashboardURL"`
	PanelURL    string                 `json:"panelURL"`
	ValueString string                 `json:"valueString"`
}

// GrafanaWebhook represents Grafana's webhook payload structure
type GrafanaWebhook struct {
	Receiver     string                 `json:"receiver"`
	Status       string                 `json:"status"`
	Alerts       []GrafanaAlert         `json:"alerts"`
	GroupLabels  map[string]string      `json:"groupLabels"`
	CommonLabels map[string]string      `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL  string                 `json:"externalURL"`
	Version      string                 `json:"version"`
	GroupKey     string                 `json:"groupKey"`
	OrgId        int                    `json:"orgId"`
	Title        string                 `json:"title"`
	State        string                 `json:"state"`
	Message      string                 `json:"message"`
}

// WebhookForwarderPlugin is the gotify plugin instance.
type WebhookForwarderPlugin struct {
	msgHandler plugin.MessageHandler
	userCtx    plugin.UserContext
}

// SetMessageHandler implements plugin.Messenger
func (p *WebhookForwarderPlugin) SetMessageHandler(h plugin.MessageHandler) {
	p.msgHandler = h
}

// GetDisplay implements plugin.Displayer to show instructions to users
func (p *WebhookForwarderPlugin) GetDisplay(location *url.URL) string {
	baseURL := ""
	if location != nil {
		baseURL = fmt.Sprintf("%s://%s", location.Scheme, location.Host)
	}
	
	pluginPath := "/plugin/YOUR_PLUGIN_ID/custom/YOUR_USER_TOKEN"
	if location != nil && location.Path != "" {
		// Extract the full plugin path from display URL
		// Path format: /plugin/{id}/custom/{token}/display
		if strings.Contains(location.Path, "/display") {
			pluginPath = strings.TrimSuffix(location.Path, "/display")
		}
	}
	
	return fmt.Sprintf(`# Webhook Forwarder Plugin

## Webhook Endpoint
POST %s%s/message

## Usage

### Generic Webhooks
Send JSON payload with message content:

` + "```bash" + `
curl -X POST %s%s/message \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Alert Title", 
    "message": "Your message here",
    "priority": 5
  }'
` + "```" + `

### Grafana Integration
1. In Grafana, go to Alerting > Contact Points
2. Add new contact point with type "webhook"
3. Set URL to: %s%s/message
4. Set method to POST
5. Save configuration

Grafana alerts are automatically detected and formatted with appropriate priority levels.

## Supported Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| message | Yes | - | Message content |
| title | No | "Webhook Message" | Message title |
| priority | No | 5 | Priority level (1-10) |
| extras | No | {} | Custom data |

## Priority Levels
- 1-2: Low (resolved alerts)
- 3-5: Normal
- 6-8: High (firing alerts) 
- 9-10: Critical

Grafana alerts automatically receive priority 8 when firing and priority 3 when resolved.

Active user: %s`, 
		baseURL, pluginPath, baseURL, pluginPath, baseURL, pluginPath, p.userCtx.Name)
}

// Enable enables the plugin.
func (p *WebhookForwarderPlugin) Enable() error {
	return nil
}

// Disable disables the plugin.
func (p *WebhookForwarderPlugin) Disable() error {
	return nil
}

// RegisterWebhook implements plugin.Webhooker.
func (p *WebhookForwarderPlugin) RegisterWebhook(basePath string, g *gin.RouterGroup) {
	// Register POST endpoint to receive webhook messages
	g.POST("/message", p.handleWebhookMessage)
	
	// Register GET endpoint for testing/info
	g.GET("/", p.handleInfo)
}

// handleWebhookMessage processes incoming webhook messages
func (p *WebhookForwarderPlugin) handleWebhookMessage(c *gin.Context) {
	// Ensure we never panic and always return a response
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error occurred while processing webhook",
				"details": "Plugin encountered an unexpected error",
			})
		}
	}()
	
	// Validate content type
	contentType := c.GetHeader("Content-Type")
	if contentType != "application/json" && contentType != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Content-Type must be application/json",
		})
		return
	}
	
	// Try to detect if this is a Grafana webhook
	var rawBody map[string]interface{}
	if err := c.ShouldBindJSON(&rawBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid JSON payload",
			"details": err.Error(),
		})
		return
	}
	
	// Safety check for nil body
	if rawBody == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Empty request body",
		})
		return
	}
	
	// Check if this looks like a Grafana webhook (has alerts field)
	if _, hasAlerts := rawBody["alerts"]; hasAlerts {
		p.handleGrafanaWebhook(c, rawBody)
		return
	}
	
	// Otherwise, treat as generic webhook
	p.handleGenericWebhook(c, rawBody)
}

// handleGenericWebhook processes standard webhook messages
func (p *WebhookForwarderPlugin) handleGenericWebhook(c *gin.Context, rawBody map[string]interface{}) {
	// Add panic recovery for this handler too
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error processing generic webhook",
				"details": "Unexpected error in webhook processing",
			})
		}
	}()
	
	var webhookMsg WebhookMessage
	
	// Safely convert map to WebhookMessage struct with type assertions
	if title, ok := rawBody["title"].(string); ok {
		webhookMsg.Title = title
	}
	if message, ok := rawBody["message"].(string); ok {
		webhookMsg.Message = message
	}
	// Handle both int and float64 for priority (JSON numbers are float64)
	if priority, ok := rawBody["priority"].(float64); ok {
		webhookMsg.Priority = int(priority)
	} else if priority, ok := rawBody["priority"].(int); ok {
		webhookMsg.Priority = priority
	}
	if extras, ok := rawBody["extras"].(map[string]interface{}); ok {
		webhookMsg.Extras = extras
	}
	
	// Validate required fields
	if webhookMsg.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Message field is required",
		})
		return
	}
	
	// Set default title if not provided
	if webhookMsg.Title == "" {
		webhookMsg.Title = "Webhook Message"
	}
	
	// Set default priority if not provided (0) or invalid
	if webhookMsg.Priority <= 0 || webhookMsg.Priority > 10 {
		webhookMsg.Priority = 5
	}
	
	// Forward message to Gotify user
	if p.msgHandler != nil {
		err := p.msgHandler.SendMessage(plugin.Message{
			Title:    webhookMsg.Title,
			Message:  webhookMsg.Message,
			Priority: webhookMsg.Priority,
			Extras:   webhookMsg.Extras,
		})
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to forward message",
				"details": err.Error(),
			})
			return
		}
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Message handler not available",
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message forwarded successfully",
	})
}

// handleGrafanaWebhook processes Grafana alert webhooks
func (p *WebhookForwarderPlugin) handleGrafanaWebhook(c *gin.Context, rawBody map[string]interface{}) {
	// Add panic recovery for Grafana webhook processing
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error processing Grafana webhook",
				"details": "Unexpected error in Grafana webhook processing",
			})
		}
	}()
	
	var grafanaMsg GrafanaWebhook
	
	// Safely extract fields from the raw body map with type assertions
	if title, ok := rawBody["title"].(string); ok {
		grafanaMsg.Title = title
	}
	if message, ok := rawBody["message"].(string); ok {
		grafanaMsg.Message = message
	}
	if status, ok := rawBody["status"].(string); ok {
		grafanaMsg.Status = status
	}
	if state, ok := rawBody["state"].(string); ok {
		grafanaMsg.State = state
	}
	
	// Determine priority based on Grafana alert status
	priority := 5
	if grafanaMsg.Status == "firing" || grafanaMsg.State == "alerting" {
		priority = 8  // High priority for firing alerts
	} else if grafanaMsg.Status == "resolved" || grafanaMsg.State == "ok" {
		priority = 3  // Lower priority for resolved alerts
	}
	
	// Use Grafana's title if available, otherwise construct one
	title := grafanaMsg.Title
	if title == "" {
		if grafanaMsg.Status != "" {
			title = "Grafana Alert: " + grafanaMsg.Status
		} else {
			title = "Grafana Alert"
		}
	}
	
	// Use Grafana's message if available
	message := grafanaMsg.Message
	if message == "" {
		message = "Alert notification from Grafana"
	}
	
	// Build extras with relevant Grafana data
	extras := make(map[string]interface{})
	extras["source"] = "grafana"
	if grafanaMsg.Status != "" {
		extras["status"] = grafanaMsg.Status
	}
	if grafanaMsg.State != "" {
		extras["state"] = grafanaMsg.State
	}
	if externalURL, ok := rawBody["externalURL"].(string); ok && externalURL != "" {
		extras["externalURL"] = externalURL
	}
	if dashboardURL, ok := rawBody["dashboardURL"].(string); ok && dashboardURL != "" {
		extras["dashboardURL"] = dashboardURL
	}
	if silenceURL, ok := rawBody["silenceURL"].(string); ok && silenceURL != "" {
		extras["silenceURL"] = silenceURL
	}
	
	// Forward message to Gotify user
	if p.msgHandler != nil {
		err := p.msgHandler.SendMessage(plugin.Message{
			Title:    title,
			Message:  message,
			Priority: priority,
			Extras:   extras,
		})
		
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to forward Grafana alert",
				"details": err.Error(),
			})
			return
		}
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Message handler not available",
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Grafana alert forwarded successfully",
		"type": "grafana",
	})
}

// handleInfo provides information about the webhook endpoint
func (p *WebhookForwarderPlugin) handleInfo(c *gin.Context) {
	// Add panic recovery for info endpoint
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Error generating plugin info",
				"details": "Unexpected error occurred",
			})
		}
	}()
	
	info := gin.H{
		"plugin": "Webhook Forwarder",
		"version": "1.0.0", 
		"user": p.userCtx.Name,
		"features": []string{
			"Generic webhook support",
			"Grafana webhook auto-detection", 
			"Smart priority assignment",
			"URL preservation for Grafana alerts",
		},
		"endpoints": gin.H{
			"send_message": gin.H{
				"method": "POST",
				"path": c.Request.URL.Path + "message",
				"description": "Send a message to this Gotify user. Supports both generic webhooks and Grafana alerts.",
				"generic_payload": gin.H{
					"title": "string (optional, defaults to 'Webhook Message')",
					"message": "string (required)",
					"priority": "int (optional, 1-10, default: 5)",
					"extras": "object (optional, custom data)",
				},
				"grafana_support": "Automatically detected when 'alerts' field is present. Priority auto-assigned: firing=8, resolved=3",
				"example_generic": gin.H{
					"title": "System Alert",
					"message": "Disk usage exceeded 90%",
					"priority": 8,
				},
				"example_curl": fmt.Sprintf("curl -X POST %s%smessage -H 'Content-Type: application/json' -d '{\"message\":\"Test alert\",\"priority\":5}'", 
					c.Request.Host, c.Request.URL.Path),
			},
			"info": gin.H{
				"method": "GET",
				"path": c.Request.URL.Path,
				"description": "Get this plugin information and usage examples",
			},
		},
	}
	
	c.JSON(http.StatusOK, info)
}

// NewGotifyPluginInstance creates a plugin instance for a user context.
func NewGotifyPluginInstance(ctx plugin.UserContext) plugin.Plugin {
	return &WebhookForwarderPlugin{
		userCtx: ctx,
	}
}

func main() {
	panic("this should be built as go plugin")
}
