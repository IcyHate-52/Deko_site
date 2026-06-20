package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Submission struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type apiError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

var (
	emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	phoneRe = regexp.MustCompile(`^[0-9+\-\s()]{7,}$`)

	fileMu      sync.Mutex
	submitsPath = "./submissions.jsonl"
)

func home(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/landing.html")
}

func bio(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/bio.html")
}

func achievements(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/achivements.html")
}

func teamHistory(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/teamhistory.html")
}

func validateSubmission(s Submission) []apiError {
	var errs []apiError

	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, apiError{"name", "Please enter your name."})
	}
	if !emailRe.MatchString(s.Email) {
		errs = append(errs, apiError{"email", "Please enter a valid email address."})
	}
	if !phoneRe.MatchString(s.Phone) {
		errs = append(errs, apiError{"phone", "Please enter a valid phone number."})
	}
	if strings.TrimSpace(s.Message) == "" {
		errs = append(errs, apiError{"message", "Please enter a message."})
	}
	return errs
}

func saveSubmission(s Submission) error {
	fileMu.Lock()
	defer fileMu.Unlock()

	f, err := os.OpenFile(submitsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func notifyByEmail(s Submission) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	to := os.Getenv("NOTIFY_EMAIL")

	if host == "" || port == "" || user == "" || pass == "" || to == "" {
		fmt.Println("Email not configured (set SMTP_HOST/SMTP_PORT/SMTP_USER/SMTP_PASS/NOTIFY_EMAIL) — skipping notification.")
		return nil
	}

	subject := "New form submission"
	body := fmt.Sprintf(
		"Name: %s\nEmail: %s\nPhone: %s\nMessage: %s\nReceived: %s",
		s.Name, s.Email, s.Phone, s.Message, s.CreatedAt.Format(time.RFC1123),
	)
	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n\r\n" +
		body + "\r\n")

	auth := smtp.PlainAuth("", user, pass, host)
	addr := host + ":" + port
	return smtp.SendMail(addr, auth, user, []string{to}, msg)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var s Submission
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	s.CreatedAt = time.Now()

	if errs := validateSubmission(s); len(errs) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{"errors": errs})
		return
	}

	if err := saveSubmission(s); err != nil {
		fmt.Println("Ошибка сохранения:", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "could not save submission"})
		return
	}

	go func() {
		if err := notifyByEmail(s); err != nil {
			fmt.Println("Ошибка отправки письма:", err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", home)
	http.HandleFunc("/bio", bio)
	http.HandleFunc("/achievements", achievements)
	http.HandleFunc("/team-history", teamHistory)
	http.HandleFunc("/submit", submitHandler)

	// ========== ЕДИНСТВЕННОЕ ИЗМЕНЕНИЕ ==========
	// Читаем порт, который назначил Render (или оставляем 8080 локально)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Сервер запущен на :%s\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Ошибка запуска:", err)
	}
}
