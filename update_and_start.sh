#!/bin/bash

# Detect package manager
if command -v pnpm &> /dev/null
then
    echo "pnpm detected. Using pnpm."
    INSTALL_CMD="pnpm install"
    BUILD_CMD="pnpm build"
    START_CMD="pnpm start"
else
    echo "pnpm not found. Falling back to npm."
    INSTALL_CMD="npm install"
    BUILD_CMD="npm run build"
    START_CMD="npm start"
fi

echo "Pulling latest changes from Git..."
git pull

echo "Installing dependencies..."
$INSTALL_CMD

echo "Building the project..."
$BUILD_CMD

echo "Starting the application..."
$START_CMD
