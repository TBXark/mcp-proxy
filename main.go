package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var BuildVersion = "dev"

func main() {
	conf := flag.String("config", "config.json", "path to config file or a http(s) url")
	insecure := flag.Bool("insecure", false, "allow insecure HTTPS connections by skipping TLS certificate verification")
	version := flag.Bool("version", false, "print version and exit")
	help := flag.Bool("help", false, "print help and exit")
	ejectTemplates := flag.Bool("eject-templates", false, "eject OAuth templates to templates/oauth/ directory for customization")
	ejectTemplatesTo := flag.String("eject-templates-to", "", "eject OAuth templates to specified directory (overrides config templateDir)")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *version {
		fmt.Println(BuildVersion)
		return
	}
	if *ejectTemplates || *ejectTemplatesTo != "" {
		var templateDir string
		
		if *ejectTemplatesTo != "" {
			// Use specified directory directly
			templateDir = *ejectTemplatesTo
		} else {
			// Load config to get templateDir if configured
			config, err := load(*conf, *insecure)
			if err != nil {
				log.Printf("Warning: Failed to load config for template directory: %v", err)
				log.Printf("Using default templates directory")
				templateDir = "templates"
			} else if config.McpProxy.Options != nil && config.McpProxy.Options.OAuth2 != nil && config.McpProxy.Options.OAuth2.TemplateDir != "" {
				templateDir = config.McpProxy.Options.OAuth2.TemplateDir
			} else {
				templateDir = "templates"
			}
		}
		
		if err := ejectOAuthTemplates(templateDir); err != nil {
			log.Fatalf("Failed to eject templates: %v", err)
		}
		return
	}
	config, err := load(*conf, *insecure)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	err = startHTTPServer(config)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func ejectOAuthTemplates(baseTemplateDir string) error {
	templatesDir := filepath.Join(baseTemplateDir, "oauth")
	
	// Create templates directory
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return fmt.Errorf("failed to create templates directory: %v", err)
	}
	
	// Write authorize.html
	authorizePath := filepath.Join(templatesDir, "authorize.html")
	if err := os.WriteFile(authorizePath, []byte(defaultAuthorizePage), 0644); err != nil {
		return fmt.Errorf("failed to write authorize.html: %v", err)
	}
	
	// Write success.html
	successPath := filepath.Join(templatesDir, "success.html")
	if err := os.WriteFile(successPath, []byte(defaultSuccessPage), 0644); err != nil {
		return fmt.Errorf("failed to write success.html: %v", err)
	}
	
	fmt.Printf("OAuth templates ejected to %s/\n", templatesDir)
	fmt.Println("You can now customize the HTML templates and restart the server to use them.")
	fmt.Println()
	fmt.Println("Template files created:")
	fmt.Printf("  %s - OAuth authorization/login page\n", authorizePath)
	fmt.Printf("  %s - OAuth success/redirect page\n", successPath)
	fmt.Println()
	fmt.Println("To use the built-in templates again, simply remove the templates/ directory.")
	
	return nil
}
