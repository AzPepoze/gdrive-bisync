package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"gdrive-bisync/internal/services/logger"
)

const serviceTemplate = `[Unit]
Description=gdrive-bisync - Google Drive Bidirectional Sync
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=default.target
`

// InstallService installs the systemd user service
func InstallService() error {
	if runtime.GOOS == "windows" {
		fmt.Print(`
╔════════════════════════════════════════════════════════════╗
║       Service Installation Not Available on Windows       ║
╚════════════════════════════════════════════════════════════╝

Systemd is not available on Windows.

To run gdrive-bisync automatically on Windows:

1. Using Task Scheduler:
   - Open Task Scheduler
   - Create a new task to run on login
   - Set the program to: gdrive-bisync.exe
   
2. Using Startup Folder:
   - Press Win+R, type: shell:startup
   - Create a shortcut to gdrive-bisync.exe there

3. Or simply run it manually when needed

For more information, visit:
  https://github.com/AzPepoze/gdrive-bisync
`)
		return fmt.Errorf("systemd service installation is not supported on Windows")
	}

	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Get user systemd directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(systemdDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %w", err)
	}

	servicePath := filepath.Join(systemdDir, "gdrive-bisync.service")

	// Create service file
	serviceContent := fmt.Sprintf(serviceTemplate, execPath)
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	logger.Info("Systemd service file created", "path", servicePath)

	// Reload systemd daemon
	cmd := exec.Command("systemctl", "--user", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("Failed to reload systemd daemon", "error", err, "output", string(output))
		logger.Info("You may need to run: systemctl --user daemon-reload")
	} else {
		logger.Info("Systemd daemon reloaded")
	}

	fmt.Printf(`
╔════════════════════════════════════════════════════════════╗
║        gdrive-bisync Service Installed Successfully        ║
╚════════════════════════════════════════════════════════════╝

Service file created at:
  %s

To enable and start the service:
  systemctl --user enable --now gdrive-bisync

To check service status:
  systemctl --user status gdrive-bisync

To view logs in real-time:
  journalctl --user -u gdrive-bisync -f

`, servicePath)

	return nil
}

// UninstallService uninstalls the systemd user service
func UninstallService() error {
	if runtime.GOOS == "windows" {
		fmt.Print(`
╔════════════════════════════════════════════════════════════╗
║      Service Uninstallation Not Available on Windows      ║
╚════════════════════════════════════════════════════════════╝

Systemd is not available on Windows.

If you set up auto-start using Task Scheduler or Startup folder,
you'll need to remove it manually from those locations.
`)
		return fmt.Errorf("systemd service uninstallation is not supported on Windows")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	servicePath := filepath.Join(home, ".config", "systemd", "user", "gdrive-bisync.service")

	// Check if service file exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return fmt.Errorf("service file not found at: %s", servicePath)
	}

	// Try to stop and disable the service
	logger.Info("Stopping and disabling service...")
	stopCmd := exec.Command("systemctl", "--user", "stop", "gdrive-bisync")
	if output, err := stopCmd.CombinedOutput(); err != nil {
		logger.Warn("Failed to stop service (may not be running)", "output", string(output))
	}

	disableCmd := exec.Command("systemctl", "--user", "disable", "gdrive-bisync")
	if output, err := disableCmd.CombinedOutput(); err != nil {
		logger.Warn("Failed to disable service (may not be enabled)", "output", string(output))
	}

	// Remove service file
	if err := os.Remove(servicePath); err != nil {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	logger.Info("Service file removed", "path", servicePath)

	// Reload systemd daemon
	cmd := exec.Command("systemctl", "--user", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("Failed to reload systemd daemon", "error", err, "output", string(output))
		logger.Info("You may need to run: systemctl --user daemon-reload")
	} else {
		logger.Info("Systemd daemon reloaded")
	}

	fmt.Printf(`
╔════════════════════════════════════════════════════════════╗
║       gdrive-bisync Service Uninstalled Successfully       ║
╚════════════════════════════════════════════════════════════╝

Service file removed from:
  %s

`, servicePath)

	return nil
}
