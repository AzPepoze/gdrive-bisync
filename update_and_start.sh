#!/bin/bash

echo "Pulling latest changes from Git..."
git pull

echo "Installing dependencies..."
pnpm install

echo "Building the project..."
pnpm build

echo "Starting the application..."
pnpm start
