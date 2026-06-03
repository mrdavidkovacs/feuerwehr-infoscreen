package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigRequiresMainURLAndDefaults(t *testing.T) {
	t.Setenv("MAIN_URL", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected missing MAIN_URL to fail")
	}

	t.Setenv("MAIN_URL", "http://example.test/status")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if cfg.MainURL != "http://example.test/status" {
		t.Fatalf("MainURL = %q", cfg.MainURL)
	}
	if cfg.ImageDir != "/app/images" || cfg.Port != "8080" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.StatusTimeout != 5*time.Second || cfg.SlideshowInterval != 15 || cfg.ImageRefresh != 60 {
		t.Fatalf("unexpected duration defaults: %#v", cfg)
	}
}

func TestImageListingFiltersAndSortsSupportedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.webp", "a.JPG", "notes.txt", "c.png", "subdir"} {
		path := filepath.Join(dir, name)
		if name == "subdir" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	imgs, err := listImages(dir)
	if err != nil {
		t.Fatalf("listImages error: %v", err)
	}
	want := []string{"a.JPG", "b.webp", "c.png"}
	if len(imgs) != len(want) {
		t.Fatalf("got %v, want %v", imgs, want)
	}
	for i := range want {
		if imgs[i] != want[i] {
			t.Fatalf("got %v, want %v", imgs, want)
		}
	}
}

func TestImagesHandlerRejectsTraversalAndServesImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := newApp(Config{MainURL: "http://example.test", ImageDir: dir, StatusTimeout: time.Second})

	bad := httptest.NewRecorder()
	app.routes().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/images/%2e%2e%2fsecret.txt", nil))
	if bad.Code != http.StatusBadRequest && bad.Code != http.StatusNotFound {
		t.Fatalf("traversal status = %d", bad.Code)
	}

	good := httptest.NewRecorder()
	app.routes().ServeHTTP(good, httptest.NewRequest(http.MethodGet, "/images/ok.png", nil))
	if good.Code != http.StatusOK {
		t.Fatalf("image status = %d", good.Code)
	}
	if good.Header().Get("Cache-Control") == "" {
		t.Fatal("missing image cache header")
	}
}

func TestStatusUsesHeadAndKeepsLastSuccess(t *testing.T) {
	var headCount int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			headCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected method %s", r.Method)
	}))
	defer upstream.Close()

	app := newApp(Config{MainURL: upstream.URL, ImageDir: t.TempDir(), StatusTimeout: time.Second})
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d", rec.Code)
	}
	if headCount != 1 {
		t.Fatalf("HEAD count = %d", headCount)
	}
	var payload StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Online || payload.LastSuccess == "" || payload.CheckedAt == "" {
		t.Fatalf("bad payload: %#v", payload)
	}
}

func TestStatusFallsBackToGetOnHeadFailure(t *testing.T) {
	var gotGet bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodGet {
			gotGet = true
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected method %s", r.Method)
	}))
	defer upstream.Close()

	app := newApp(Config{MainURL: upstream.URL, ImageDir: t.TempDir(), StatusTimeout: time.Second})
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d", rec.Code)
	}
	var payload StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !gotGet || !payload.Online {
		t.Fatalf("fallback failed: gotGet=%v payload=%#v", gotGet, payload)
	}
}
