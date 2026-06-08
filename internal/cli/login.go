package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the FlowCtl server via OIDC",
	Long:  "Opens a browser for OIDC authentication and saves the token to ~/.flowctl/config.yaml.",
	RunE:  runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Start a local HTTP server on a random port to receive the OIDC callback
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	tokenCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			errCh <- fmt.Errorf("no token received in callback")
			http.Error(w, "Authentication failed: no token received", http.StatusBadRequest)
			return
		}
		tokenCh <- token
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h2>Authentication successful!</h2><p>You can close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	// Construct the OIDC login URL
	loginURL := fmt.Sprintf("%s/auth/login?redirect_uri=%s", cfgServer, callbackURL)

	fmt.Printf("Opening browser for authentication...\n")
	fmt.Printf("If the browser does not open, visit:\n  %s\n\n", loginURL)

	// Open browser
	if err := openBrowser(loginURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
	}

	// Wait for callback or timeout
	fmt.Println("Waiting for authentication...")
	select {
	case token := <-tokenCh:
		// Gracefully shut down the server
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)

		// Save token to config
		if err := saveToken(token); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		// Fetch user info to display email
		cfgToken = token
		client := NewAPIClient()
		var userInfo struct {
			Email string `json:"email"`
		}
		if err := client.Get("/auth/userinfo", &userInfo); err != nil {
			// If we can't get user info, just confirm login
			fmt.Println("Successfully logged in.")
			return nil
		}
		fmt.Printf("Successfully logged in as %s\n", userInfo.Email)
		return nil

	case err := <-errCh:
		return err

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("authentication timed out after 5 minutes")
	}
}

func saveToken(token string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	configDir := filepath.Join(home, ".flowctl")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	viper.Set("token", token)
	viper.SetConfigFile(configPath)
	if err := viper.WriteConfig(); err != nil {
		// If the file does not exist, SafeWriteConfig creates it
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
	}
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
