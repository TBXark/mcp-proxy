#!/bin/bash

# MCP Proxy Example Launcher
# This script helps you quickly start the proxy with different example configurations

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration directory
EXAMPLES_DIR="$(dirname "$0")/configs"
BUILD_DIR="$(dirname "$0")/../build"
PROXY_BINARY="$BUILD_DIR/mcp-proxy"

# Function to print colored output
print_color() {
    printf "${1}${2}${NC}\n"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [config-name]"
    echo ""
    echo "Available configurations:"
    echo "  simple           - Simple two-server setup (memory + filesystem)"
    echo "  comprehensive    - Complete setup with all server types"
    echo "  custom           - Use your own config.json file"
    echo ""
    echo "Examples:"
    echo "  $0 simple                    # Start with simple configuration"
    echo "  $0 comprehensive             # Start with comprehensive configuration"
    echo "  $0 custom                    # Start with ./config.json"
    echo ""
}

# Function to check if binary exists
check_binary() {
    if [ ! -f "$PROXY_BINARY" ]; then
        print_color $RED "❌ MCP Proxy binary not found at: $PROXY_BINARY"
        print_color $YELLOW "Please build the proxy first:"
        print_color $BLUE "  make build"
        exit 1
    fi
}

# Function to validate config
validate_config() {
    local config_file="$1"
    
    if [ ! -f "$config_file" ]; then
        print_color $RED "❌ Configuration file not found: $config_file"
        exit 1
    fi
    
    # Basic JSON validation
    if ! python3 -m json.tool "$config_file" >/dev/null 2>&1; then
        print_color $RED "❌ Invalid JSON in configuration file: $config_file"
        exit 1
    fi
    
    print_color $GREEN "✅ Configuration file is valid"
}

# Function to start proxy
start_proxy() {
    local config_file="$1"
    local config_name="$2"
    
    print_color $BLUE "🚀 Starting MCP Proxy with $config_name configuration..."
    print_color $YELLOW "Configuration: $config_file"
    print_color $YELLOW "Proxy URL: http://localhost:9090"
    print_color $YELLOW ""
    print_color $YELLOW "Press Ctrl+C to stop the proxy"
    print_color $YELLOW ""
    
    exec "$PROXY_BINARY" --config "$config_file"
}

# Main script
main() {
    local config_name="${1:-}"
    
    # Show usage if no arguments or help requested
    if [ -z "$config_name" ] || [ "$config_name" = "-h" ] || [ "$config_name" = "--help" ]; then
        show_usage
        exit 0
    fi
    
    # Check if binary exists
    check_binary
    
    # Determine config file
    case "$config_name" in
        "simple")
            config_file="$EXAMPLES_DIR/simple-example.json"
            display_name="Simple"
            ;;
        "comprehensive")
            config_file="$EXAMPLES_DIR/comprehensive-example.json"
            display_name="Comprehensive"
            ;;
        "custom")
            config_file="./config.json"
            display_name="Custom"
            ;;
        *)
            print_color $RED "❌ Unknown configuration: $config_name"
            show_usage
            exit 1
            ;;
    esac
    
    # Validate and start
    validate_config "$config_file"
    start_proxy "$config_file" "$display_name"
}

# Run main function with all arguments
main "$@"
