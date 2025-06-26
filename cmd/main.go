package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/thanksduck/email_service/internal/api"
	"github.com/thanksduck/email_service/internal/emailservice"
	"github.com/thanksduck/email_service/internal/envcheck"
)

func main() {
	fmt.Println("Initialising the email service")
	envcheck.Init()

	// Initialize the EmailService without templatesDir
	service := emailservice.NewEmailService()
	defer service.Stop()

	// Initialize the API handlers with the EmailService
	emailAPI := api.NewEmailAPI(service)

	// Register both endpoints
	http.HandleFunc("GET /send", emailAPI.HandleGetSendEmail)
	http.HandleFunc("POST /send", emailAPI.HandlePostSendEmail)

	port := os.Getenv("EMAIL_SERVICE_PORT")
	if port == "" {
		port = "7979"
	}
	fmt.Printf("Starting server on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
