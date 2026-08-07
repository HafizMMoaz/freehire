//go:build integration

// Integration test: GET /api/v1/geo/cities is reachable through the real route
// wiring (handler.Register), unauthenticated, and its country filter narrows
// results — the part a unit test against the isolated geoApp() helper cannot
// prove, since that helper skips Register entirely. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestSearchCitiesEndpoint_ReachableThroughRegister(t *testing.T) {
	pool := startPostgres(t)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	Register(app, Config{Pool: pool, FrontendOrigin: "http://localhost", JWTSecret: "test-secret", JWTTTL: time.Hour})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/geo/cities?q=Florian", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (city search must be reachable and unauthenticated)", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			Value   string `json:"value"`
			Country string `json:"country"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, m := range body.Data {
		if m.Value == "Florianópolis" && m.Country == "br" {
			found = true
		}
	}
	if !found {
		t.Errorf("data = %+v, missing Florianópolis/br", body.Data)
	}
}

func TestSearchCitiesEndpoint_CountryFilterThroughRegister(t *testing.T) {
	pool := startPostgres(t)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	Register(app, Config{Pool: pool, FrontendOrigin: "http://localhost", JWTSecret: "test-secret", JWTTTL: time.Hour})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/geo/cities?q=San&country=us", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			Value   string `json:"value"`
			Country string `json:"country"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) == 0 {
		t.Fatal("expected at least one match for q=San&country=us")
	}
	for _, m := range body.Data {
		if m.Country != "us" {
			t.Errorf("result %+v has country %q, want us (filter not applied through Register)", m, m.Country)
		}
	}
}
