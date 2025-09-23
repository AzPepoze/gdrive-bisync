#!/bin/bash

# This script creates and installs a systemd service for the gdrive-bisync application.

# Stop execution if any command fails
set -e

SERVICE_NAME="gdrive-bisync"
SERVICE_FILE="$SERVICE_NAME.service"
UPDATE_SCRIPT="update_and_start.sh"
USER=$(whoami)
GROUP=$(id -gn $USER)
WORKING_DIR=$(pwd)

# Make the update script executable
chmod +x "$UPDATE_SCRIPT"

echo "Creating systemd service file for $SERVICE_NAME..."

# Create the service file content
cat > "$SERVICE_FILE" << EOL
[Unit]
Description=gdrive-bisync - A custom Google Drive sync client.
After=network.target

[Service]
Type=simple
User=$USER
Group=$GROUP
WorkingDirectory=$WORKING_DIR
ExecStart=$WORKING_DIR/$UPDATE_SCRIPT
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOL

echo "Service file created at $WORKING_DIR/$SERVICE_FILE"

echo "Installing and starting the service..."

# Move the service file, reload daemon, enable and start the service
sudo mv "$SERVICE_FILE" "/etc/systemd/system/$SERVICE_FILE"
sudo systemctl daemon-reload
sudo systemctl enable "$SERVICE_NAME"
sudo systemctl start "$SERVICE_NAME"

echo ""
echo "Service '$SERVICE_NAME' has been created, enabled, and started successfully."
echo "To check the status, run: sudo systemctl status $SERVICE_NAME"
echo "To see the logs, you may need to check the system's journal with: journalctl -u $SERVICE_NAME"
echo "To stop the service, run: sudo systemctl stop $SERVICE_NAME"