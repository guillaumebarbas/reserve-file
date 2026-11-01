package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	const swaggerUIOrigin = "http://localhost:8081"
	testCases := []struct {
		name              string
		allowedOrigin     string
		method            string
		origin            string
		requestedMethod   string
		wantStatus        int
		wantAllowOrigin   string
		wantAllowMethods  string
		wantAllowHeaders  string
		wantExposeHeaders string
		wantVary          string
	}{
		{
			name:             "preflight_from_the_allowed_origin",
			allowedOrigin:    swaggerUIOrigin,
			method:           http.MethodOptions,
			origin:           swaggerUIOrigin,
			requestedMethod:  http.MethodPost,
			wantStatus:       http.StatusNoContent,
			wantAllowOrigin:  swaggerUIOrigin,
			wantAllowMethods: "GET, POST",
			wantAllowHeaders: "Content-Type, Idempotency-Key",
			wantVary:         "Origin",
		},
		{
			name:              "request_from_the_allowed_origin",
			allowedOrigin:     swaggerUIOrigin,
			method:            http.MethodPost,
			origin:            swaggerUIOrigin,
			wantStatus:        http.StatusTeapot,
			wantAllowOrigin:   swaggerUIOrigin,
			wantExposeHeaders: "Idempotent-Replayed, Location, Retry-After, X-Request-Id",
			wantVary:          "Origin",
		},
		{
			name:            "preflight_from_another_origin",
			allowedOrigin:   swaggerUIOrigin,
			method:          http.MethodOptions,
			origin:          "http://attacker.example",
			requestedMethod: http.MethodPost,
			wantStatus:      http.StatusTeapot,
			wantVary:        "Origin",
		},
		{
			name:          "request_from_another_origin",
			allowedOrigin: swaggerUIOrigin,
			method:        http.MethodPost,
			origin:        "http://attacker.example",
			wantStatus:    http.StatusTeapot,
			wantVary:      "Origin",
		},
		{
			name:            "nothing_configured",
			allowedOrigin:   "",
			method:          http.MethodOptions,
			origin:          swaggerUIOrigin,
			requestedMethod: http.MethodPost,
			wantStatus:      http.StatusTeapot,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(http.StatusTeapot)
			})
			request := httptest.NewRequest(testCase.method, "/accounts/large_account/reservations", nil)
			request.Header.Set("Origin", testCase.origin)
			if testCase.requestedMethod != "" {
				request.Header.Set("Access-Control-Request-Method", testCase.requestedMethod)
			}
			recorder := httptest.NewRecorder()

			WithCORS(testCase.allowedOrigin, nextHandler).ServeHTTP(recorder, request)

			got := corsResponse{
				status:        recorder.Code,
				allowOrigin:   recorder.Header().Get("Access-Control-Allow-Origin"),
				allowMethods:  recorder.Header().Get("Access-Control-Allow-Methods"),
				allowHeaders:  recorder.Header().Get("Access-Control-Allow-Headers"),
				exposeHeaders: recorder.Header().Get("Access-Control-Expose-Headers"),
				vary:          recorder.Header().Get("Vary"),
			}
			want := corsResponse{
				status:        testCase.wantStatus,
				allowOrigin:   testCase.wantAllowOrigin,
				allowMethods:  testCase.wantAllowMethods,
				allowHeaders:  testCase.wantAllowHeaders,
				exposeHeaders: testCase.wantExposeHeaders,
				vary:          testCase.wantVary,
			}
			if got != want {
				t.Errorf("%s from %q with %q allowed:\n got  %+v\n want %+v", testCase.method, testCase.origin, testCase.allowedOrigin, got, want)
			}
		})
	}
}

type corsResponse struct {
	status        int
	allowOrigin   string
	allowMethods  string
	allowHeaders  string
	exposeHeaders string
	vary          string
}
