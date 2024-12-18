package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the script configuration
type Config struct {
	ignoredExtensions map[string]bool
	ignoredFiles      map[string]bool
}

func main() {
	// Define flags
	sourcePath := flag.String("source", ".", "Source directory path")
	outputPath := flag.String("output", "repository_contents.txt", "Output file path")
	ignoreExts := flag.String("ignore-ext", ".git,.gitignore,.idea,node_modules,.vscode,.env,.txt", "Comma-separated list of extensions/directories to ignore")
	ignoreFiles := flag.String("ignore-files", "go.sum,cmd/repo2text/main.go,README.md,.DS_Store", "Comma-separated list of specific files to ignore")
	flag.Parse()

	// Initialize config
	config := &Config{
		ignoredExtensions: make(map[string]bool),
		ignoredFiles:      make(map[string]bool),
	}

	// Process ignored extensions
	for _, ext := range strings.Split(*ignoreExts, ",") {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			config.ignoredExtensions[ext] = true
		}
	}

	// Process ignored files
	for _, file := range strings.Split(*ignoreFiles, ",") {
		file = strings.TrimSpace(file)
		if file != "" {
			config.ignoredFiles[file] = true
		}
	}

	// Create output file
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outputFile.Close()

	// Process the repository
	err = processRepository(*sourcePath, outputFile, config)
	if err != nil {
		fmt.Printf("Error processing repository: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Repository contents written to %s\n", *outputPath)
}

func processRepository(sourcePath string, output io.Writer, config *Config) error {
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if it's a directory
		if info.IsDir() {
			// Check if directory should be ignored
			if config.shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be ignored
		if config.shouldIgnore(info.Name()) {
			return nil
		}

		// Read file contents
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", path, err)
		}

		// Write file path and contents to output
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("error getting relative path for %s: %w", path, err)
		}

		// Write file header and contents
		_, err = fmt.Fprintf(output, "// File: %s\n\n%s\n\n", relPath, string(content))
		if err != nil {
			return fmt.Errorf("error writing to output file: %w", err)
		}

		return nil
	})
}

func (c *Config) shouldIgnore(name string) bool {
	// Check if the file/directory is in the ignored files list
	if c.ignoredFiles[name] {
		return true
	}

	// Check if the file/directory extension is in the ignored extensions list
	ext := filepath.Ext(name)
	if c.ignoredExtensions[ext] {
		return true
	}

	// Check if the entire name matches any ignored extension
	// This handles directories like .git, node_modules, etc.
	if c.ignoredExtensions[name] {
		return true
	}

	return false
}
