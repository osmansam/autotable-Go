#!/bin/bash

# Load Testing Tool Installer
# This script installs all necessary load testing tools

set -e

echo "🚀 Installing Load Testing Tools..."
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go first."
    exit 1
fi

echo "✅ Go is installed: $(go version)"
echo ""

# Install hey
echo "📦 Installing hey..."
if command -v hey &> /dev/null; then
    echo "✅ hey is already installed: $(hey -version 2>&1 | head -n1)"
else
    go install github.com/rakyll/hey@latest
    echo "✅ hey installed successfully"
fi
echo ""

# Install vegeta
echo "📦 Installing vegeta..."
if command -v vegeta &> /dev/null; then
    echo "✅ vegeta is already installed: $(vegeta -version)"
else
    if command -v brew &> /dev/null; then
        brew install vegeta
    else
        go install github.com/tsenart/vegeta@latest
    fi
    echo "✅ vegeta installed successfully"
fi
echo ""

# Install k6
echo "📦 Installing k6..."
if command -v k6 &> /dev/null; then
    echo "✅ k6 is already installed: $(k6 version)"
else
    if command -v brew &> /dev/null; then
        brew install k6
    else
        echo "⚠️  Homebrew not found. Please install k6 manually from https://k6.io/docs/getting-started/installation/"
    fi
fi
echo ""

echo "🎉 Installation complete!"
echo ""
echo "Installed tools:"
command -v hey &> /dev/null && echo "  ✅ hey: $(which hey)"
command -v vegeta &> /dev/null && echo "  ✅ vegeta: $(which vegeta)"
command -v k6 &> /dev/null && echo "  ✅ k6: $(which k6)"
echo ""
echo "You can now run load tests!"
