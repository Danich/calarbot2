package botModules

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ServeModule starts an HTTP server that exposes the given BotModule's functionality.
// It returns the server instance and an error channel. The caller is responsible for
// shutting down the server when done by calling the server's Shutdown method.
func ServeModule(module BotModule, addr string) (*http.Server, <-chan error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/order", orderAction(module))
	mux.HandleFunc("/is_called", isCalledAction(module))
	mux.HandleFunc("/answer", answerAction(module))

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start the server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.ListenAndServe()
	}()

	return server, errChan
}

func answerAction(module BotModule) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var msg Payload
		err := json.NewDecoder(r.Body).Decode(&msg)
		if err != nil {
			log.Println(err)
		}
		answer, err := module.Answer(&msg)
		resp := map[string]interface{}{"answer": answer}
		if err != nil {
			resp["error"] = err.Error()
		}
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			resp["error"] = err.Error()
		}
	}
}

func isCalledAction(module BotModule) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload Payload
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err != nil {
			fmt.Printf("error decoding payload: %v", err)
		}
		result := module.IsCalled(payload.Msg)
		err = json.NewEncoder(w).Encode(map[string]bool{"called": result})
		if err != nil {
			fmt.Printf("error encoding response: %v", err)
		}
	}
}

func orderAction(module BotModule) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		order := module.Order()
		err := json.NewEncoder(w).Encode(map[string]int{"order": order})
		if err != nil {
			fmt.Printf("error encoding order: %v", err)
		}
	}
}

// RunModuleServer starts an HTTP server for a BotModule and handles graceful shutdown.
// It takes a BotModule, an address string, and an optional shutdown timeout duration.
// If timeout is 0, a default of 5 seconds is used.
// This function blocks until the server is shut down or an error occurs.
func RunModuleServer(module BotModule, addr string, timeout time.Duration) error {
	// Use default timeout if not specified
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// Start the server
	server, errChan := ServeModule(module, addr)

	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for termination signal or server error
	select {
	case <-stop:
		fmt.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("error during server shutdown: %w", err)
		}
		return nil
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}
