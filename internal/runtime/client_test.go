package runtime

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

func TestRetryableReadIncludesDocumentedSearchPosts(t *testing.T) {
	for _, path := range []string{"/api/v2/audit/events/search", "/api/v2/logs/events/search", "/api/v2/logs/analytics/aggregate"} {
		request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: path}}
		if !retryableRead(request) {
			t.Fatalf("expected %s POST to be retryable", path)
		}
	}
	request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: "/api/v1/monitor"}}
	if retryableRead(request) {
		t.Fatal("monitor mutation-like POST must not be retryable")
	}
}

func TestRetryTransportRetriesSearchPostAndRewindsBody(t *testing.T) {
	var calls int
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		bodies = append(bodies, string(body))
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	payload := []byte(`{"query":"service:web","from":"now-1h"}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v2/spans/events/search", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	transport := retryTransport{base: http.DefaultTransport, attempts: 3}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("retry transport: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(bodies) != 2 || bodies[0] != string(payload) || bodies[1] != string(payload) {
		t.Fatalf("request body was not replayed losslessly: %#v", bodies)
	}
}

func TestRetryTransportDoesNotRetryMutationPost(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/monitor/123", bytes.NewReader([]byte(`{"name":"changed"}`)))
	if err != nil {
		t.Fatal(err)
	}
	response, err := (retryTransport{base: http.DefaultTransport, attempts: 3}).RoundTrip(request)
	if err != nil {
		t.Fatalf("mutation request: %v", err)
	}
	defer response.Body.Close()
	if calls != 1 {
		t.Fatalf("mutation POST was retried %d times", calls)
	}
}

func TestWrapAPIErrorDoesNotExposeResponseBody(t *testing.T) {
	err := WrapAPIError(datadog.GenericOpenAPIError{ErrorMessage: "500 Internal Server Error", ErrorBody: []byte(`{"api_key":"secret-value"}`)})
	if strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "500 Internal Server Error") {
		t.Fatalf("unexpected sanitized API error: %v", err)
	}
}

func TestWrapAPIErrorIncludesDecodedMessagesWithoutRawBody(t *testing.T) {
	err := WrapAPIError(datadog.GenericOpenAPIError{
		ErrorMessage: "400 Bad Request",
		ErrorBody:    []byte(`{"errors":["invalid metric query"]}`),
		ErrorModel:   datadogV1.NewAPIErrorResponse([]string{"invalid metric query"}),
	})
	if !strings.Contains(err.Error(), "invalid metric query") || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("unexpected decoded API error: %v", err)
	}
}

func TestWrapAPIErrorRedactsEnvFileCredentials(t *testing.T) {
	t.Setenv("DD_API_KEY", "process-api")
	t.Setenv("DD_APP_KEY", "process-app")
	err := WrapAPIError(datadog.GenericOpenAPIError{ErrorMessage: "request rejected process-api env-api process-app env-app"}, Config{APIKey: "env-api", AppKey: "env-app"})
	if strings.Contains(err.Error(), "process-api") || strings.Contains(err.Error(), "process-app") || strings.Contains(err.Error(), "env-api") || strings.Contains(err.Error(), "env-app") {
		t.Fatalf("credentials leaked in API error: %v", err)
	}
}

func TestSanitizeErrorRedactsShortCredentials(t *testing.T) {
	t.Setenv("DD_API_KEY", "x")
	if got := SanitizeError("credential=x"); got != "credential=[REDACTED]" {
		t.Fatalf("short credential was not redacted: %q", got)
	}
}

func TestWrapAPIErrorClassifiesCancellation(t *testing.T) {
	if got := WrapAPIError(context.DeadlineExceeded).Error(); got != "datadog API request timed out" {
		t.Fatalf("unexpected timeout error: %s", got)
	}
}

func TestRetryDelayHonorsRetryAfter(t *testing.T) {
	ctx := context.Background()
	response := &http.Response{Header: http.Header{"Retry-After": []string{"61"}}}
	if _, ok := retryDelay(response, 1, ctx); ok {
		t.Fatal("expected long Retry-After to skip retry")
	}
	response.Header.Set("Retry-After", "0")
	if wait, ok := retryDelay(response, 1, ctx); !ok || wait != 0 {
		t.Fatalf("expected immediate retry, wait=%s ok=%t", wait, ok)
	}
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Millisecond))
	defer cancel()
	response.Header.Set("Retry-After", "1")
	if _, ok := retryDelay(response, 1, deadlineCtx); ok {
		t.Fatal("expected retry to respect context deadline")
	}
}
