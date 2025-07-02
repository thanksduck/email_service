package emailservice

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"sync"
	"time"
)

type EmailData struct {
	To           string
	Subject      string
	Body         string
	Data         map[string]interface{}
	SMTPServer   string // Optional: for dynamic SMTP credentials
	SMTPPort     int    // Optional: for dynamic SMTP credentials
	SMTPUsername string // Optional: for dynamic SMTP credentials
	SMTPPassword string // Optional: for dynamic SMTP credentials
	UseSSL       bool   // Optional: force SSL/TLS usage
	UseTLS       bool   // Optional: force STARTTLS usage
}

type EmailService struct {
	smtpHost    string
	smtpPort    string
	smtpUser    string
	smtpPass    string
	senderEmail string
	useSSL      bool // Default SSL setting
	useTLS      bool // Default TLS setting
	emailQueue  chan EmailData
	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewEmailService() *EmailService {
	ctx, cancel := context.WithCancel(context.Background())

	// Parse SSL/TLS settings from environment
	useSSL, _ := strconv.ParseBool(os.Getenv("SMTP_USE_SSL"))
	useTLS, _ := strconv.ParseBool(os.Getenv("SMTP_USE_TLS"))

	// Auto-detect SSL based on port if not explicitly set
	smtpPort := os.Getenv("SMTP_PORT")
	if !useSSL && !useTLS && smtpPort == "465" {
		useSSL = true
	} else if !useSSL && !useTLS && smtpPort == "587" {
		useTLS = true
	}

	service := &EmailService{
		smtpHost:    os.Getenv("SMTP_HOST"),
		smtpPort:    smtpPort,
		smtpUser:    os.Getenv("SMTP_USER"),
		smtpPass:    os.Getenv("SMTP_PASS"),
		senderEmail: os.Getenv("SENDER_EMAIL"),
		useSSL:      useSSL,
		useTLS:      useTLS,
		emailQueue:  make(chan EmailData, 100), // Buffer size for the queue
		workerCount: 5,                         // Number of concurrent workers
		ctx:         ctx,
		cancel:      cancel,
	}

	service.startWorkers()

	return service
}

func (s *EmailService) startWorkers() {
	for i := 0; i < s.workerCount; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
}

func (s *EmailService) worker(id int) {
	defer s.wg.Done()

	log.Printf("Email worker %d started", id)

	for {
		select {
		case email, ok := <-s.emailQueue:
			if !ok {
				log.Printf("Email worker %d stopping: queue closed", id)
				return
			}
			if err := s.sendEmail(email); err != nil {
				log.Printf("Email worker %d - Failed to send email to %s: %v", id, email.To, err)
				// Log detailed error information
				s.logDetailedError(email, err)
			} else {
				log.Printf("Email worker %d - Successfully sent email to %s", id, email.To)
			}
		case <-s.ctx.Done():
			log.Printf("Email worker %d stopping: context cancelled", id)
			return
		}
	}
}

func (s *EmailService) logDetailedError(email EmailData, err error) {
	log.Printf("=== EMAIL DELIVERY FAILURE DETAILS ===")
	log.Printf("Recipient: %s", email.To)
	log.Printf("Subject: %s", email.Subject)
	log.Printf("SMTP Server: %s", s.getEffectiveSMTPHost(email))
	log.Printf("SMTP Port: %s", s.getEffectiveSMTPPort(email))
	log.Printf("Username: %s", s.getEffectiveSMTPUser(email))
	log.Printf("SSL Enabled: %t", s.getEffectiveSSL(email))
	log.Printf("TLS Enabled: %t", s.getEffectiveTLS(email))
	log.Printf("Error: %v", err)
	log.Printf("Timestamp: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("=====================================")
}

func (s *EmailService) getEffectiveSMTPHost(data EmailData) string {
	if data.SMTPServer != "" {
		return data.SMTPServer
	}
	return s.smtpHost
}

func (s *EmailService) getEffectiveSMTPPort(data EmailData) string {
	if data.SMTPPort != 0 {
		return fmt.Sprintf("%d", data.SMTPPort)
	}
	return s.smtpPort
}

func (s *EmailService) getEffectiveSMTPUser(data EmailData) string {
	if data.SMTPUsername != "" {
		return data.SMTPUsername
	}
	return s.smtpUser
}

func (s *EmailService) getEffectiveSMTPPass(data EmailData) string {
	if data.SMTPPassword != "" {
		return data.SMTPPassword
	}
	return s.smtpPass
}

func (s *EmailService) getEffectiveSSL(data EmailData) bool {
	// If SSL is explicitly set in email data, use that
	if data.UseSSL {
		return true
	}
	// Auto-detect based on port
	port := s.getEffectiveSMTPPort(data)
	if port == "465" {
		return true
	}
	return s.useSSL
}

func (s *EmailService) getEffectiveTLS(data EmailData) bool {
	// If TLS is explicitly set in email data, use that
	if data.UseTLS {
		return true
	}
	// Auto-detect based on port
	port := s.getEffectiveSMTPPort(data)
	if port == "587" {
		return true
	}
	return s.useTLS
}

func (s *EmailService) sendEmail(data EmailData) error {
	// Get effective SMTP settings
	smtpHost := s.getEffectiveSMTPHost(data)
	smtpPort := s.getEffectiveSMTPPort(data)
	smtpUser := s.getEffectiveSMTPUser(data)
	smtpPass := s.getEffectiveSMTPPass(data)
	useSSL := s.getEffectiveSSL(data)
	useTLS := s.getEffectiveTLS(data)

	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)

	// Determine sender email
	senderEmail := s.senderEmail
	if senderEmail == "" && smtpUser != "" {
		senderEmail = smtpUser
	}

	log.Printf("Sending email to %s using %s (SSL: %t, TLS: %t)", data.To, addr, useSSL, useTLS)

	// Prepare message
	headers := map[string]string{
		"From":         senderEmail,
		"To":           data.To,
		"Subject":      data.Subject,
		"MIME-version": "1.0",
		"Content-Type": "text/html; charset=\"UTF-8\"",
	}

	var message bytes.Buffer
	for k, v := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n")
	message.WriteString(data.Body)

	to := []string{data.To}

	// Handle different connection types
	if useSSL {
		return s.sendEmailSSL(addr, smtpUser, smtpPass, senderEmail, to, message.Bytes())
	} else if useTLS {
		return s.sendEmailTLS(addr, smtpHost, smtpUser, smtpPass, senderEmail, to, message.Bytes())
	} else {
		// Plain SMTP
		auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
		return smtp.SendMail(addr, auth, senderEmail, to, message.Bytes())
	}
}

func (s *EmailService) sendEmailSSL(addr, username, password, from string, to []string, msg []byte) error {
	// Create TLS connection for SSL (port 465)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         addr[:len(addr)-4], // Remove :port part
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to establish SSL connection: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, tlsConfig.ServerName)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	// Authenticate
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, tlsConfig.ServerName)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = writer.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return nil
}

func (s *EmailService) sendEmailTLS(addr, hostname, username, password, from string, to []string, msg []byte) error {
	// Connect to SMTP server
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Quit()

	// Start TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         hostname,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Authenticate
	if username != "" && password != "" {
		auth := smtp.PlainAuth("", username, password, hostname)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = writer.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return nil
}

func (s *EmailService) Stop() {
	log.Println("Stopping email service...")
	s.cancel()          // Signal workers to stop
	close(s.emailQueue) // Close the queue to prevent new items
	s.wg.Wait()         // Wait for all workers to finish
	log.Println("Email service stopped gracefully.")
}

func (s *EmailService) QueueEmail(data EmailData) error {
	select {
	case s.emailQueue <- data:
		log.Printf("Email queued successfully for %s", data.To)
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("email service is stopping, cannot queue new email")
	default:
		return fmt.Errorf("email queue is full, try again later")
	}
}

// Helper to execute the template for the email body
func ExecuteTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("emailTemplate").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := tmpl.Execute(&bodyBuf, data); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}
	return bodyBuf.String(), nil
}

// TestConnection - Helper function to test SMTP connection
func (s *EmailService) TestConnection(data EmailData) error {
	smtpHost := s.getEffectiveSMTPHost(data)
	smtpPort := s.getEffectiveSMTPPort(data)
	smtpUser := s.getEffectiveSMTPUser(data)
	smtpPass := s.getEffectiveSMTPPass(data)
	useSSL := s.getEffectiveSSL(data)
	useTLS := s.getEffectiveTLS(data)

	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)

	log.Printf("Testing connection to %s (SSL: %t, TLS: %t)", addr, useSSL, useTLS)

	if useSSL {
		// Test SSL connection
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         smtpHost,
		}

		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("SSL connection test failed: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, smtpHost)
		if err != nil {
			return fmt.Errorf("SMTP client creation failed: %w", err)
		}
		defer client.Quit()

		if smtpUser != "" && smtpPass != "" {
			auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("authentication test failed: %w", err)
			}
		}

		log.Println("SSL connection test successful!")
		return nil
	} else if useTLS {
		// Test TLS connection
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("SMTP connection test failed: %w", err)
		}
		defer client.Quit()

		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         smtpHost,
		}

		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("TLS start test failed: %w", err)
		}

		if smtpUser != "" && smtpPass != "" {
			auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("authentication test failed: %w", err)
			}
		}

		log.Println("TLS connection test successful!")
		return nil
	} else {
		// Test plain connection
		auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("plain SMTP connection test failed: %w", err)
		}
		defer client.Quit()

		if smtpUser != "" && smtpPass != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("authentication test failed: %w", err)
			}
		}

		log.Println("Plain SMTP connection test successful!")
		return nil
	}
}
