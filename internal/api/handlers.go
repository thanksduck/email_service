package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/thanksduck/email_service/internal/emailservice"
)

type EmailService struct {
	Service *emailservice.EmailService
}

// PlaceholderValue represents a key-value pair for template data.
type PlaceholderValue struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"` // Using interface{} to accept any type
}

// SendEmailRequest structure for POST /send
type SendEmailRequest struct {
	To              string             `json:"to"`
	Subject         string             `json:"subject"`
	Template        string             `json:"template"` // The template content itself
	PlaceholderData []PlaceholderValue `json:"placeholders"`
	// Optional SMTP credentials
	SMTPServer   string `json:"smtp_server,omitempty"`
	SMTPPort     int    `json:"smtp_port,omitempty"`
	SMTPUsername string `json:"smtp_username,omitempty"`
	SMTPPassword string `json:"smtp_password,omitempty"`
}

// GetEmailRequest structure for GET /send
type GetEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Slug    string `json:"slug"` // For GET, 'slug' acts as a placeholder for a simple template name.
	Data    string `json:"data"` // JSON string for template data
}

// NewEmailAPI creates a new EmailService API handler.
func NewEmailAPI(service *emailservice.EmailService) *EmailService {
	return &EmailService{
		Service: service,
	}
}

// handleGetSendEmail handles GET /send requests.
// It's simplified to demonstrate the removal of file-based templates.
// In a real scenario, you might have a predefined set of simple templates,
// or deprecate this GET endpoint entirely in favor of POST.
func (es *EmailService) HandleGetSendEmail(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Get required parameters
	to := query.Get("to")
	if to == "" {
		http.Error(w, "Missing 'to' parameter", http.StatusBadRequest)
		return
	}

	// For GET, the "template" logic is simplified.
	// You might hardcode a very basic template here, or define a few simple ones.
	// For this refactor, we'll use a very basic placeholder template for GET.
	// In a real application, you'd likely want to remove or drastically change this GET endpoint.
	slug := query.Get("slug") // Still accepting slug for compatibility, but it won't load a file.
	templateContent := fmt.Sprintf("<h1>Hello!</h1><p>This is a default email for %s.</p>", slug)

	// Get optional subject parameter
	subject := query.Get("subject")
	if subject == "" {
		subject = fmt.Sprintf("Notification for %s", slug) // Default subject
	}

	// Initialize template data
	data := make(map[string]interface{})

	// Check if we have a JSON data block
	jsonData := query.Get("data")
	if jsonData != "" {
		// Parse the JSON data
		var parsedData map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &parsedData); err != nil {
			http.Error(w, "Invalid JSON in 'data' parameter", http.StatusBadRequest)
			return
		}

		// Add all JSON fields to our template data
		for key, value := range parsedData {
			data[key] = value
		}
	}

	// Execute template to get the email body
	body, err := emailservice.ExecuteTemplate(templateContent, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
		return
	}

	// Queue the email for sending
	emailData := emailservice.EmailData{
		To:      to,
		Subject: subject,
		Body:    body,
		Data:    data,
	}

	if err := es.Service.QueueEmail(emailData); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue email: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"success","message":"Email queued for delivery (GET request)"}`)
}

// HandlePostSendEmail handles POST /send requests.
func (es *EmailService) HandlePostSendEmail(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var req SendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("Error decoding request: %v\n", err) // More verbose logging
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate required fields
	if req.To == "" {
		http.Error(w, "Missing 'to' field", http.StatusBadRequest)
		return
	}
	if req.Template == "" {
		http.Error(w, "Missing 'template' field (HTML content)", http.StatusBadRequest)
		return
	}

	// Set default subject if not provided
	if req.Subject == "" {
		req.Subject = "Email Notification"
	}

	// Convert placeholder values to a map for template execution
	data := make(map[string]interface{})
	for _, p := range req.PlaceholderData {
		data[p.Key] = p.Value
	}

	// Execute template to get the email body
	body, err := emailservice.ExecuteTemplate(req.Template, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %v", err), http.StatusInternalServerError)
		return
	}

	// Queue the email for sending
	emailData := emailservice.EmailData{
		To:           req.To,
		Subject:      req.Subject,
		Body:         body,
		Data:         data, // This is still passed, but not strictly used by sendEmail itself unless for logging/debugging
		SMTPServer:   req.SMTPServer,
		SMTPPort:     req.SMTPPort,
		SMTPUsername: req.SMTPUsername,
		SMTPPassword: req.SMTPPassword,
	}

	if err := es.Service.QueueEmail(emailData); err != nil {
		http.Error(w, fmt.Sprintf("Failed to queue email: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"success","message":"Email queued for delivery (POST request)"}`)
}
