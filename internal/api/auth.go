package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"gdrive-bisync/internal/services/logger"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const (
	TokenPath       = "config/token.json"
	CredentialsPath = "config/credentials.json"
)

type AuthorizedUser struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// Authorize returns an authorized Drive client.
func Authorize(ctx context.Context) (*http.Client, error) {
	b, err := os.ReadFile(TokenPath)
	if err != nil {
		logger.Error("Authentication failed: Could not load token.", "path", TokenPath, "error", err)
		return nil, fmt.Errorf("authentication not configured: %w", err)
	}

	creds, err := google.CredentialsFromJSON(ctx, b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("invalid token file: %w", err)
	}

	return oauth2.NewClient(ctx, creds.TokenSource), nil
}

// SetupAuthentication performs the OAuth2 flow and saves the token.
func SetupAuthentication(ctx context.Context) error {
	logger.Info("--- Google Drive Authentication Setup ---")

	b, err := os.ReadFile(CredentialsPath)
	if err != nil {
		logger.Error(fmt.Sprintf("Error: '%s' not found.", CredentialsPath))
		fmt.Printf(`
1. Go to the Google Cloud Console: https://console.cloud.google.com/
2. Create a new project or select an existing one.
3. In the library, search for and enable the "Google Drive API".
4. Go to "Credentials" -> "Create Credentials" -> "OAuth client ID".
5. Select "Desktop app" as the application type.
6. Download the JSON file provided after creation.
7. IMPORTANT: Rename the downloaded file to "credentials.json" and place it in the "config" directory of this project.
`)
		return err
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return fmt.Errorf("unable to parse client secret file to config: %w", err)
	}

	client := getClient(ctx, config)
	if client == nil {
		return fmt.Errorf("failed to get client")
	}

	logger.Info("Authentication successful! Token saved.")
	return nil
}

// getClient uses the config to generate a token, then saves it.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {

	// Listen on a random free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Error("Failed to start local auth server", "error", err)
		return nil
	}
	defer listener.Close()

	// Update redirect URL with the assigned port
	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	codeChan := make(chan string)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<h1>Authentication Successful!</h1><p>You can close this window and return to the terminal.</p>"))
			codeChan <- code
		} else {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<h1>Authentication Failed</h1><p>No code received.</p>"))
		}
	})

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("Auth server error", "error", err)
		}
	}()
	defer server.Shutdown(ctx)

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	logger.Info("Starting auth server", "port", port)
	fmt.Printf("\nOpening browser for authentication...\nIf it doesn't open, visit this link manually:\n%v\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		logger.Warn("Failed to open browser", "error", err)
	}

	authCode := <-codeChan

	tok, err := config.Exchange(ctx, authCode)
	if err != nil {
		logger.Error("Unable to retrieve token from web", "error", err)
		return nil
	}

	payload := AuthorizedUser{
		Type:         "authorized_user",
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RefreshToken: tok.RefreshToken,
	}

	f, err := os.Create(TokenPath)
	if err != nil {
		logger.Error("Unable to cache oauth token", "error", err)
		return nil
	}
	defer f.Close()
	json.NewEncoder(f).Encode(payload)

	return config.Client(ctx, tok)
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
