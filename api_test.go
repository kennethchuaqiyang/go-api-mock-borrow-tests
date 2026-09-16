// Package mockborrowtests contains black-box API automation tests for the
// live mock-borrow-api service (https://mock-borrow-api.onrender.com).
//
// Unlike main_test.go in the server repo (which calls handler functions
// directly, in-process), these tests are true end-to-end HTTP tests: they
// make real requests over the network against the deployed service, the
// same way a QA engineer's automation suite would test any live API.
//
// Note: the service runs on Render's free tier, which spins down after
// inactivity. The first request in a run may take 20-30s to respond while
// it wakes up — the http.Client timeout below is set generously to allow
// for this.
package mockborrowtests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"testing"
	"time"
)

const baseURL = "https://mock-borrow-api.onrender.com"

var httpClient = &http.Client{
	Timeout: 40 * time.Second, // generous, to allow for Render free-tier cold starts
}

var sha256HexPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// freshUserID returns a randomized userid so tests don't interfere with
// each other's balances when run against the shared live database.
func freshUserID() int {
	return 100000 + rand.Intn(900000)
}

// --- helpers ---

type getResponse struct {
	Metadata struct {
		Username string  `json:"username"`
		Location string  `json:"location"`
		Salary   float64 `json:"salary"`
	} `json:"metadata"`
	Cache struct {
		Username     string  `json:"username"`
		UserIdentity int     `json:"user_identity"`
		Salary       float64 `json:"salary"`
	} `json:"cache"`
}

type postResponse struct {
	Metadata struct {
		Username        string  `json:"username"`
		Location        string  `json:"location"`
		AmountOwed      float64 `json:"amount_owed"`
		AmountAlteredBy float64 `json:"amount_altered_to_borrow"`
		AllowedToBorrow bool    `json:"allowed_to_borrow"`
	} `json:"metadata"`
}

func doGet(t *testing.T, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func doPost(t *testing.T, path string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s) // allows passing deliberately-invalid raw JSON as a string
	} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("failed to encode request body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+path, &buf)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
}

// --- GET /api/user ---

func TestGetUser_HappyPath(t *testing.T) {
	resp := doGet(t, "/api/user?username=john&location=Singapore&userid=1", map[string]string{
		"X-Browser-Type": "Chrome",
		"X-Admin-Flag":   "true",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body getResponse
	decode(t, resp, &body)

	if body.Metadata.Username != "john" {
		t.Errorf("expected metadata username 'john', got %q", body.Metadata.Username)
	}
	if body.Metadata.Location != "Singapore" {
		t.Errorf("expected metadata location 'Singapore', got %q", body.Metadata.Location)
	}
	if body.Metadata.Salary != 5000 {
		t.Errorf("expected metadata salary 5000, got %v", body.Metadata.Salary)
	}
	if body.Cache.UserIdentity != 1 {
		t.Errorf("expected cache user_identity 1, got %d", body.Cache.UserIdentity)
	}
	if body.Cache.Salary != 5000 {
		t.Errorf("expected cache salary 5000, got %v", body.Cache.Salary)
	}
}

func TestGetUser_ResponseHeaders(t *testing.T) {
	resp := doGet(t, "/api/user?username=john&location=Singapore&userid=1", map[string]string{
		"X-Browser-Type": "Chrome",
	})
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Browser"); got != "Chrome" {
		t.Errorf("expected X-Browser header 'Chrome', got %q", got)
	}

	secretKey := resp.Header.Get("X-Secret-Key")
	if secretKey == "" {
		t.Fatal("expected non-empty X-Secret-Key header")
	}
	if !sha256HexPattern.MatchString(secretKey) {
		t.Errorf("expected X-Secret-Key to look like a sha256 hex digest, got %q", secretKey)
	}
}

func TestGetUser_MissingParams_Returns400(t *testing.T) {
	resp := doGet(t, "/api/user?location=Singapore", nil) // username and userid omitted
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetUser_NonIntegerUserID_Returns400(t *testing.T) {
	resp := doGet(t, "/api/user?username=john&location=Singapore&userid=not-a-number", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetUser_WrongMethod_Returns405(t *testing.T) {
	resp := doPost(t, "/api/user", map[string]interface{}{}, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

// --- POST /api/borrow ---

func TestBorrow_HappyPath(t *testing.T) {
	userID := freshUserID()

	resp := doPost(t, "/api/borrow", map[string]interface{}{
		"username":         "testuser",
		"location":         "Singapore",
		"userid":           userID,
		"amount_to_borrow": 50,
	}, map[string]string{"X-Browser-Type": "Chrome", "X-Admin-Flag": "false"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body postResponse
	decode(t, resp, &body)

	if body.Metadata.AmountOwed != 50 {
		t.Errorf("expected amount_owed 50, got %v", body.Metadata.AmountOwed)
	}
	if body.Metadata.AmountAlteredBy != 50 {
		t.Errorf("expected amount_altered_to_borrow 50, got %v", body.Metadata.AmountAlteredBy)
	}
	if !body.Metadata.AllowedToBorrow {
		t.Error("expected allowed_to_borrow true")
	}
}

func TestBorrow_OverLimit_RejectedAndUnchanged(t *testing.T) {
	userID := freshUserID()

	// First borrow: pushes the user to 150, still within limit.
	first := doPost(t, "/api/borrow", map[string]interface{}{
		"username": "testuser", "location": "Singapore", "userid": userID, "amount_to_borrow": 150,
	}, nil)
	first.Body.Close()

	// Second borrow: 150 + 100 = 250, exceeds the 200 limit.
	resp := doPost(t, "/api/borrow", map[string]interface{}{
		"username": "testuser", "location": "Singapore", "userid": userID, "amount_to_borrow": 100,
	}, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body postResponse
	decode(t, resp, &body)

	if body.Metadata.AllowedToBorrow {
		t.Error("expected allowed_to_borrow false")
	}
	if body.Metadata.AmountAlteredBy != 0 {
		t.Errorf("expected amount_altered_to_borrow 0, got %v", body.Metadata.AmountAlteredBy)
	}
	if body.Metadata.AmountOwed != 150 {
		t.Errorf("expected amount_owed to remain 150, got %v", body.Metadata.AmountOwed)
	}
}

func TestBorrow_ExactlyAtLimit_StillAllowed(t *testing.T) {
	userID := freshUserID()

	resp := doPost(t, "/api/borrow", map[string]interface{}{
		"username": "testuser", "location": "Singapore", "userid": userID, "amount_to_borrow": 200,
	}, nil)
	defer resp.Body.Close()

	var body postResponse
	decode(t, resp, &body)

	if !body.Metadata.AllowedToBorrow {
		t.Error("expected allowed_to_borrow true for exactly 200 (only 'above' 200 is rejected)")
	}
	if body.Metadata.AmountOwed != 200 {
		t.Errorf("expected amount_owed 200, got %v", body.Metadata.AmountOwed)
	}
}

func TestBorrow_MissingUsername_Returns400(t *testing.T) {
	resp := doPost(t, "/api/borrow", map[string]interface{}{
		"location": "Singapore", "userid": freshUserID(), "amount_to_borrow": 50,
	}, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBorrow_InvalidJSON_Returns400(t *testing.T) {
	resp := doPost(t, "/api/borrow", "{not valid json", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBorrow_WrongMethod_Returns405(t *testing.T) {
	resp := doGet(t, "/api/borrow", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestBorrow_ResponseHeaders(t *testing.T) {
	resp := doPost(t, "/api/borrow", map[string]interface{}{
		"username": "testuser", "location": "Singapore", "userid": freshUserID(), "amount_to_borrow": 10,
	}, map[string]string{"X-Browser-Type": "Firefox"})
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Browser"); got != "Firefox" {
		t.Errorf("expected X-Browser header 'Firefox', got %q", got)
	}
	if secretKey := resp.Header.Get("X-Secret-Key"); !sha256HexPattern.MatchString(secretKey) {
		t.Errorf("expected X-Secret-Key to look like a sha256 hex digest, got %q", secretKey)
	}
}

func init() {
	// seed once so freshUserID() doesn't produce the same sequence every run
	rand.Seed(time.Now().UnixNano())
	fmt.Println("running API automation tests against:", baseURL)
}