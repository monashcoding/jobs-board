package main

import (
	"flag"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config holds the script configuration
type Config struct {
	ignoredExtensions map[string]bool
	ignoredFiles      map[string]bool
	outputDir         string
}

var defaultIgnoredFiles = []string{
	"cmd/repo-export/main.go",
	"go.sum",
}

var defaultIgnoredExtensions = []string{
	".git",
	".html",
	".gitignore",
	".idea",
	"node_modules",
	".vscode",
	".env",
	".DS_Store",
}

// PackageFiles maps package paths to their files
type PackageFiles struct {
	files map[string][]string // package path -> files
}

func newPackageFiles() *PackageFiles {
	return &PackageFiles{
		files: make(map[string][]string),
	}
}

func (pf *PackageFiles) addFile(path string) {
	// Get the first level of the directory
	parts := strings.Split(path, string(filepath.Separator))

	// Skip files in root directory
	if len(parts) == 1 {
		return
	}

	// Take only first directory level (e.g., "internal-scraper" for "internal/scraper/sites/file.go")
	packageName := strings.ReplaceAll(parts[0], string(filepath.Separator), "-")
	if len(parts) > 2 {
		packageName += "-" + parts[1]
	}

	// Initialize slice if needed
	if _, exists := pf.files[packageName]; !exists {
		pf.files[packageName] = []string{}
	}

	// Add file to package
	pf.files[packageName] = append(pf.files[packageName], path)
}

// Add this new function
func writeFileTree(sourcePath string, config *Config) error {
	treePath := filepath.Join(config.outputDir, "repository-files.txt")
	treeFile, err := os.Create(treePath)
	if err != nil {
		return fmt.Errorf("error creating tree file: %w", err)
	}
	defer treeFile.Close()

	// Collect and sort all files
	var allFiles []string
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if config.shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}

		if !config.shouldIgnore(info.Name()) && !config.shouldIgnore(relPath) {
			allFiles = append(allFiles, relPath)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Sort files for consistent output
	sort.Strings(allFiles)

	// Write all files
	for _, file := range allFiles {
		if _, err := fmt.Fprintln(treeFile, file); err != nil {
			return fmt.Errorf("error writing to tree file: %w", err)
		}
	}

	utils.Log("Created file tree: %s", treePath)
	return nil
}

func main() {
	// Define flags
	sourcePath := flag.String("source", ".", "Source directory path")
	outputDir := flag.String("output-dir", "repository", "Output directory for package files")
	ignoreExts := flag.String("ignore-ext", strings.Join(defaultIgnoredExtensions, ","), "Comma-separated list of extensions/directories to ignore")
	ignoreFiles := flag.String("ignore-files", strings.Join(defaultIgnoredFiles, ","), "Comma-separated list of specific files to ignore")
	flag.Parse()

	// Initialize config
	config := &Config{
		ignoredExtensions: make(map[string]bool),
		ignoredFiles:      make(map[string]bool),
		outputDir:         *outputDir,
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

	// Clean the output directory first
	err := os.RemoveAll(config.outputDir)
	if err != nil {
		fmt.Printf("Error cleaning output directory: %v\n", err)
		os.Exit(1)
	}
	utils.Log("Cleaned output directory: %s", config.outputDir)

	// Create fresh directory
	if err := os.MkdirAll(config.outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Write file tree
	if err := writeFileTree(*sourcePath, config); err != nil {
		fmt.Printf("Error creating file tree: %v\n", err)
	}

	// Collect files by package
	packages := newPackageFiles()
	err = filepath.Walk(*sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if it's a directory
		if info.IsDir() {
			if config.shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(*sourcePath, path)
		if err != nil {
			return fmt.Errorf("error getting relative path for %s: %w", path, err)
		}

		// Skip ignored files
		if config.shouldIgnore(info.Name()) || config.shouldIgnore(relPath) {
			return nil
		}

		packages.addFile(relPath)
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking repository: %v\n", err)
		os.Exit(1)
	}

	// Process each package
	for packageName, files := range packages.files {
		// Sort files for consistent output
		sort.Strings(files)

		// Create output file for package
		outputPath := filepath.Join(config.outputDir, packageName+".txt")
		outputFile, err := os.Create(outputPath)
		if err != nil {
			fmt.Printf("Error creating output file for package %s: %v\n", packageName, err)
			continue
		}

		// Write all files in the package
		for _, file := range files {
			if err := writeFileContent(*sourcePath, file, outputFile); err != nil {
				fmt.Printf("Error processing file %s: %v\n", file, err)
				continue
			}
		}

		outputFile.Close()
		utils.Log("Created package file: %s", outputPath)
	}
}

func writeFileContent(basePath, relPath string, output io.Writer) error {
	fullPath := filepath.Join(basePath, relPath)

	// Read file contents
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	// Write file header and contents
	_, err = fmt.Fprintf(output, "// File: %s\n%s\n", relPath, string(content))
	if err != nil {
		return fmt.Errorf("error writing to output: %w", err)
	}

	return nil
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
