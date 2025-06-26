package emailservice

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"os"
	"sync"
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
}

type EmailService struct {
	smtpHost    string
	smtpPort    string
	smtpUser    string
	smtpPass    string
	senderEmail string
	emailQueue  chan EmailData
	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewEmailService() *EmailService {
	ctx, cancel := context.WithCancel(context.Background())

	service := &EmailService{
		smtpHost:    os.Getenv("SMTP_HOST"),
		smtpPort:    os.Getenv("SMTP_PORT"),
		smtpUser:    os.Getenv("SMTP_USER"),
		smtpPass:    os.Getenv("SMTP_PASS"),
		senderEmail: os.Getenv("SENDER_EMAIL"),
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
				log.Printf("Failed to send email to %s: %v", email.To, err)
			}
		case <-s.ctx.Done():
			log.Printf("Email worker %d stopping: context cancelled", id)
			return
		}
	}
}

func (s *EmailService) sendEmail(data EmailData) error {
	// Determine SMTP credentials to use
	smtpHost := s.smtpHost
	smtpPort := s.smtpPort
	smtpUser := s.smtpUser
	smtpPass := s.smtpPass

	// If dynamic credentials are provided, use them
	if data.SMTPServer != "" && data.SMTPPort != 0 && data.SMTPUsername != "" && data.SMTPPassword != "" {
		smtpHost = data.SMTPServer
		smtpPort = fmt.Sprintf("%d", data.SMTPPort)
		smtpUser = data.SMTPUsername
		smtpPass = data.SMTPPassword
	}

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)

	// Determine sender email
	senderEmail := s.senderEmail
	if smtpUser != "" { // If dynamic user is provided, use it as sender if senderEmail is not explicitly set
		senderEmail = smtpUser
	}

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
	log.Printf("Attempting to send email to %s using SMTP server %s:%s", data.To, smtpHost, smtpPort)
	return smtp.SendMail(addr, auth, senderEmail, to, message.Bytes())
}

func (s *EmailService) Stop() {
	s.cancel()          // Signal workers to stop
	close(s.emailQueue) // Close the queue to prevent new items
	s.wg.Wait()         // Wait for all workers to finish
	log.Println("Email service stopped gracefully.")
}

func (s *EmailService) QueueEmail(data EmailData) error {
	select {
	case s.emailQueue <- data:
		return nil
	case <-s.ctx.Done(): // Check if the context is cancelled before queuing
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
