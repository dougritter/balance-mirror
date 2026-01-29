package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandleRoot_Index(t *testing.T) {
	// Setup templates path for tests to run from server package directory
	// We need to change cwd or absolute paths.
	// Easier is to use os.Chdir("..") if we run from server package, but that's messy.
	// Let's assume we run tests from root.

	// Better: We must ensure templates are found.
	// The server uses "templates/..." relative path.
	// If we run `go test ./server/...`, CWD is `.../server`.
	// We should probably help it find templates.
	os.Chdir("..")

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleRoot)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "Balance Mirror"
	if !strings.Contains(rr.Body.String(), expected) {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestHandleRoot_Proxy(t *testing.T) {
	// 1. Start a mock Linktree server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `
			<html>
				<body>
					<a href="https://zoom.us/j/123456" data-testid="LinkClickTriggerLink">Zoom Link</a>
					<a href="https://example.com/2" data-testid="LinkClickTriggerLink">Non-Zoom Link</a>
					<a href="#">Ignored</a>
				</body>
			</html>
		`)
	}))
	defer mockServer.Close()

	// 2. Override AllowedIDs to point to mock server
	oldIDs := AllowedIDs
	AllowedIDs = map[string]string{
		"test-id": mockServer.URL,
	}
	defer func() { AllowedIDs = oldIDs }()

	// 3. Request the page
	req, err := http.NewRequest("GET", "/test-id", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleRoot)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	body := rr.Body.String()
	// Verify that scraped links are present
	if !strings.Contains(body, "Zoom Link") {
		t.Error("Body does not contain 'Zoom Link'")
	}
	if !strings.Contains(body, "https://zoom.us/j/123456") {
		t.Error("Body does not contain 'https://zoom.us/j/123456'")
	}
	if strings.Contains(body, "Non-Zoom Link") {
		t.Error("Body contains 'Non-Zoom Link' which should be filtered out")
	}
}
