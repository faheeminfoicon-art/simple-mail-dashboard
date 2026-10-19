package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Contact struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Consented bool   `json:"consented"`
}

type Campaign struct {
	Name       string `json:"name"`
	Subject    string `json:"subject"`
	Status     string `json:"status"`
	Recipients int    `json:"recipients"`
	Sent       int    `json:"sent"`
	Failed     int    `json:"failed"`
	CreatedAt  string `json:"createdAt"`
}

type Store struct {
	Contacts  []Contact  `json:"contacts"`
	Campaigns []Campaign `json:"campaigns"`
}

type BrevoRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type BrevoSender struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type BrevoRequest struct {
	Sender      BrevoSender      `json:"sender"`
	To          []BrevoRecipient `json:"to"`
	Subject     string           `json:"subject"`
	HTMLContent string           `json:"htmlContent"`
	TextContent string           `json:"textContent,omitempty"`
}

var (
	mu sync.Mutex
	db Store
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func dataFile() string {
	return env("DATA_FILE", "data/store.json")
}

func loadStore() {
	content, err := os.ReadFile(dataFile())
	if err == nil {
		_ = json.Unmarshal(content, &db)
	}
}

func saveStore() {
	content, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(dataFile(), content, 0600)
}

func sendJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedUser := os.Getenv("ADMIN_USER")
		expectedPassword := os.Getenv("ADMIN_PASSWORD")

		username, password, ok := r.BasicAuth()

		if !ok ||
			username != expectedUser ||
			password != expectedPassword {

			w.Header().Set(
				"WWW-Authenticate",
				`Basic realm="Bulk Mail Sender"`,
			)

			http.Error(
				w,
				"Authentication required",
				http.StatusUnauthorized,
			)

			return
		}

		next(w, r)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func stateHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	sendJSON(w, db)
}

func contactsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"POST method required",
			http.StatusMethodNotAllowed,
		)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "CSV file required", http.StatusBadRequest)
		return
	}

	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) < 2 {
		http.Error(w, "Invalid CSV file", http.StatusBadRequest)
		return
	}

	headers := map[string]int{}

	for index, header := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(header))] = index
	}

	emailIndex, emailExists := headers["email"]
	if !emailExists {
		http.Error(
			w,
			"CSV must contain an email column",
			http.StatusBadRequest,
		)
		return
	}

	nameIndex, nameExists := headers["name"]
	consentIndex, consentExists := headers["consented"]

	added := 0

	mu.Lock()
	defer mu.Unlock()

	for _, row := range rows[1:] {
		if emailIndex >= len(row) {
			continue
		}

		email := strings.TrimSpace(row[emailIndex])

		if !strings.Contains(email, "@") {
			continue
		}

		name := ""

		if nameExists && nameIndex < len(row) {
			name = strings.TrimSpace(row[nameIndex])
		}

		consented := false

		if consentExists && consentIndex < len(row) {
			value := strings.ToLower(
				strings.TrimSpace(row[consentIndex]),
			)

			consented =
				value == "true" ||
					value == "yes" ||
					value == "1"
		}

		if !consented {
			continue
		}

		duplicate := false

		for _, existing := range db.Contacts {
			if strings.EqualFold(existing.Email, email) {
				duplicate = true
				break
			}
		}

		if duplicate {
			continue
		}

		db.Contacts = append(
			db.Contacts,
			Contact{
				Name:      name,
				Email:     email,
				Consented: true,
			},
		)

		added++
	}

	saveStore()

	sendJSON(
		w,
		map[string]any{
			"ok":    true,
			"added": added,
			"total": len(db.Contacts),
		},
	)
}

func sendWithBrevo(
	recipientName string,
	recipientEmail string,
	subject string,
	htmlBody string,
) error {
	apiKey := os.Getenv("BREVO_API_KEY")
	fromEmail := os.Getenv("FROM_EMAIL")
	fromName := env("FROM_NAME", "Bulk Mail Sender")

	if apiKey == "" {
		return fmt.Errorf("BREVO_API_KEY is missing")
	}

	if fromEmail == "" {
		return fmt.Errorf("FROM_EMAIL is missing")
	}

	payload := BrevoRequest{
		Sender: BrevoSender{
			Email: fromEmail,
			Name:  fromName,
		},
		To: []BrevoRecipient{
			{
				Email: recipientEmail,
				Name:  recipientName,
			},
		},
		Subject:     subject,
		HTMLContent: htmlBody,
		TextContent: "This email contains HTML content.",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(
		http.MethodPost,
		"https://api.brevo.com/v3/smtp/email",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", "application/json")
	request.Header.Set("api-key", apiKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf(
			"Brevo API error %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	return nil
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"POST method required",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var input struct {
		To string `json:"to"`
	}

	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	input.To = strings.TrimSpace(input.To)

	if !strings.Contains(input.To, "@") {
		http.Error(
			w,
			"Valid test email required",
			http.StatusBadRequest,
		)
		return
	}

	htmlBody := `
		<div style="font-family:Arial,sans-serif;padding:24px">
			<h2 style="color:#4338ca">Test successful</h2>
			<p>Your Bulk Mail Sender is connected to the Brevo Email API.</p>
			<p>This test was sent through an HTTPS API, not through SMTP.</p>
		</div>
	`

	err := sendWithBrevo(
		"Test Recipient",
		input.To,
		"Bulk Mail Sender test successful",
		htmlBody,
	)

	if err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	sendJSON(
		w,
		map[string]any{
			"ok":      true,
			"message": "Test email accepted by Brevo",
		},
	)
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"POST method required",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var input struct {
		Name           string `json:"Name"`
		Subject        string `json:"Subject"`
		Body           string `json:"Body"`
		ConfirmConsent bool   `json:"ConfirmConsent"`
	}

	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.Subject) == "" ||
		strings.TrimSpace(input.Body) == "" ||
		!input.ConfirmConsent {

		http.Error(
			w,
			"Subject, message and consent confirmation are required",
			http.StatusBadRequest,
		)

		return
	}

	mu.Lock()
	contacts := append([]Contact(nil), db.Contacts...)
	mu.Unlock()

	if len(contacts) == 0 {
		http.Error(
			w,
			"No consented recipients available",
			http.StatusBadRequest,
		)
		return
	}

	if len(contacts) > 300 {
		http.Error(
			w,
			"Maximum 300 recipients per campaign",
			http.StatusBadRequest,
		)
		return
	}

	sent := 0
	failed := 0

	for _, contact := range contacts {
		if !contact.Consented {
			continue
		}

		message := strings.ReplaceAll(
			input.Body,
			"{{name}}",
			template.HTMLEscapeString(contact.Name),
		)

		message += `
			<hr>
			<p style="font-size:12px;color:#64748b">
				You received this message because you opted in.
				Contact the sender to unsubscribe.
			</p>
		`

		err := sendWithBrevo(
			contact.Name,
			contact.Email,
			input.Subject,
			message,
		)

		if err != nil {
			log.Printf(
				"Email failed for %s: %v",
				contact.Email,
				err,
			)

			failed++
			continue
		}

		sent++
	}

	status := "Completed"

	if failed > 0 {
		status = "Partial"
	}

	campaignName := strings.TrimSpace(input.Name)
	if campaignName == "" {
		campaignName = "Campaign"
	}

	mu.Lock()

	db.Campaigns = append(
		[]Campaign{
			{
				Name:       campaignName,
				Subject:    input.Subject,
				Status:     status,
				Recipients: len(contacts),
				Sent:       sent,
				Failed:     failed,
				CreatedAt:  time.Now().Format(time.RFC3339),
			},
		},
		db.Campaigns...,
	)

	saveStore()

	mu.Unlock()

	sendJSON(
		w,
		map[string]any{
			"ok":     true,
			"sent":   sent,
			"failed": failed,
		},
	)
}

func main() {
	loadStore()

	http.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	http.HandleFunc("/healthz", healthHandler)
	http.HandleFunc("/api/state", auth(stateHandler))
	http.HandleFunc("/api/contacts", auth(contactsHandler))
	http.HandleFunc("/api/test", auth(testHandler))
	http.HandleFunc("/api/send", auth(sendHandler))

	http.HandleFunc(
		"/",
		auth(
			func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}

				http.ServeFile(
					w,
					r,
					"static/index.html",
				)
			},
		),
	)

	port := env("PORT", "10000")

	log.Println("Bulk Mail Sender running on port", port)

	log.Fatal(
		http.ListenAndServe(":"+port, nil),
	)
}
