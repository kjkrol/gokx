package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// katalog, w którym leżą index.html, main.wasm, wasm_exec.js
	baseDir := filepath.Join("cmd", "wasm-demo")

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(baseDir))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(baseDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})

	srv := &http.Server{Addr: ":8080", Handler: logRequests(mux)}

	go func() {
		log.Println("Serving on http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	// sygnały zamknięcia
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server Shutdown Failed:%+v", err)
	}

	log.Println("Server stopped")
	os.Exit(0)
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}
