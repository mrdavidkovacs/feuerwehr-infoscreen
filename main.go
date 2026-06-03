package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	MainURL           string
	ImageDir          string
	Port              string
	StatusTimeout     time.Duration
	SlideshowInterval int
	ImageRefresh      int
}

type app struct {
	cfg         Config
	client      *http.Client
	mu          sync.RWMutex
	lastSuccess time.Time
}

type StatusResponse struct {
	Online      bool   `json:"online"`
	LastSuccess string `json:"lastSuccess"`
	CheckedAt   string `json:"checkedAt"`
}

type imageListResponse struct {
	Images []string `json:"images"`
}

type clientConfigResponse struct {
	MainURL                  string `json:"mainUrl"`
	SlideshowIntervalSeconds int    `json:"slideshowIntervalSeconds"`
	ImageRefreshSeconds      int    `json:"imageRefreshSeconds"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	a := newApp(cfg)
	addr := ":" + cfg.Port
	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(a.routes()),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	log.Printf("feuerwehr-infoscreen starting on %s main_url=%s image_dir=%s", addr, cfg.MainURL, cfg.ImageDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

func loadConfig() (Config, error) {
	mainURL := strings.TrimSpace(os.Getenv("MAIN_URL"))
	if mainURL == "" {
		return Config{}, errors.New("MAIN_URL is required")
	}
	parsed, err := url.Parse(mainURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Config{}, fmt.Errorf("MAIN_URL must be an absolute URL: %q", mainURL)
	}

	statusTimeout, err := envInt("STATUS_TIMEOUT_SECONDS", 5)
	if err != nil {
		return Config{}, err
	}
	slideshow, err := envInt("SLIDESHOW_INTERVAL_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}
	refresh, err := envInt("IMAGE_REFRESH_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}

	return Config{
		MainURL:           mainURL,
		ImageDir:          envString("IMAGE_DIR", "/app/images"),
		Port:              envString("PORT", "8080"),
		StatusTimeout:     time.Duration(statusTimeout) * time.Second,
		SlideshowInterval: slideshow,
		ImageRefresh:      refresh,
	}, nil
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func newApp(cfg Config) *app {
	if cfg.StatusTimeout <= 0 {
		cfg.StatusTimeout = 5 * time.Second
	}
	return &app{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.StatusTimeout,
		},
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleRoot)
	mux.HandleFunc("/display1", a.servePublicFile("display1.html"))
	mux.HandleFunc("/display2", a.servePublicFile("display2.html"))
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/images", a.handleImages)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/images/", a.handleImageFile)
	mux.HandleFunc("/css/", a.servePublicAsset)
	mux.HandleFunc("/js/", a.servePublicAsset)
	return mux
}

func (a *app) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/display1", http.StatusFound)
}

func (a *app) servePublicFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w)
			return
		}
		setNoCache(w)
		http.ServeFile(w, r, filepath.Join("public", name))
	}
}

func (a *app) servePublicAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	setNoCache(w)
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, filepath.Join("public", clean))
}

func (a *app) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	setJSON(w)
	setNoCache(w)
	_ = json.NewEncoder(w).Encode(clientConfigResponse{
		MainURL:                  a.cfg.MainURL,
		SlideshowIntervalSeconds: a.cfg.SlideshowInterval,
		ImageRefreshSeconds:      a.cfg.ImageRefresh,
	})
}

func (a *app) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	checkedAt := time.Now().UTC()
	online := a.checkMainURL()
	if online {
		a.mu.Lock()
		a.lastSuccess = checkedAt
		a.mu.Unlock()
	}

	a.mu.RLock()
	last := a.lastSuccess
	a.mu.RUnlock()

	lastText := ""
	if !last.IsZero() {
		lastText = last.Format(time.RFC3339)
	}
	setJSON(w)
	setNoCache(w)
	_ = json.NewEncoder(w).Encode(StatusResponse{
		Online:      online,
		LastSuccess: lastText,
		CheckedAt:   checkedAt.Format(time.RFC3339),
	})
}

func (a *app) checkMainURL() bool {
	if a.tryRequest(http.MethodHead) {
		return true
	}
	return a.tryRequest(http.MethodGet)
}

func (a *app) tryRequest(method string) bool {
	req, err := http.NewRequest(method, a.cfg.MainURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "feuerwehr-infoscreen/1.0")
	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("status check %s failed: %v", method, err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func (a *app) handleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	images, err := listImages(a.cfg.ImageDir)
	if err != nil {
		log.Printf("image listing failed: %v", err)
		images = []string{}
	}
	setJSON(w)
	setNoCache(w)
	_ = json.NewEncoder(w).Encode(imageListResponse{Images: images})
}

func listImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	images := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isSupportedImage(name) {
			images = append(images, name)
		}
	}
	sort.Slice(images, func(i, j int) bool {
		return strings.ToLower(images[i]) < strings.ToLower(images[j])
	})
	return images, nil
}

func isSupportedImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func (a *app) handleImageFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	rel, ok := safeImageName(strings.TrimPrefix(r.URL.Path, "/images/"))
	if !ok || !isSupportedImage(rel) {
		http.Error(w, "bad image path", http.StatusBadRequest)
		return
	}
	path := filepath.Join(a.cfg.ImageDir, rel)
	if !isPathInside(a.cfg.ImageDir, path) {
		http.Error(w, "bad image path", http.StatusBadRequest)
		return
	}
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, path)
}

func safeImageName(raw string) (string, bool) {
	if raw == "" || strings.Contains(raw, "\x00") || strings.Contains(raw, "/") || strings.Contains(raw, "\\") {
		return "", false
	}
	clean := filepath.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", false
	}
	return clean, true
}

func isPathInside(base, candidate string) bool {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absCandidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func setJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

func setNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
