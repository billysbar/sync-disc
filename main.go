package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	var sourceDir, destDir string
	var dryRun, verbose bool

	flag.StringVar(&sourceDir, "source", "", "Source directory to copy files from")
	flag.StringVar(&destDir, "dest", "", "Destination directory to copy files to")
	flag.BoolVar(&dryRun, "dry-run", false, "Perform a dry run - show what would be copied without actually copying")
	flag.BoolVar(&verbose, "verbose", false, "Show individual files instead of directory-level summary (use with -dry-run)")
	flag.Parse()

	if sourceDir == "" || destDir == "" {
		fmt.Printf("Usage: %s -source /path/to/source -dest /path/to/destination [-dry-run] [-verbose]\n", os.Args[0])
		fmt.Println("\nThis tool copies all files from source to destination, skipping files that already exist in destination.")
		fmt.Println("Directory structure is preserved.")
		fmt.Println("\nOptions:")
		fmt.Println("  -dry-run    Show what would be copied without actually copying (shows summary by default)")
		fmt.Println("  -verbose    Show individual files instead of directory-level summary (use with -dry-run)")
		os.Exit(1)
	}

	// Validate directories exist
	if err := validateDirectory(sourceDir, "Source"); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	if err := validateDirectory(destDir, "Destination"); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📁 Source: %s\n", sourceDir)
	fmt.Printf("📁 Destination: %s\n", destDir)
	if dryRun {
		fmt.Println("🔍 DRY RUN MODE - No files will be copied")
		if !verbose {
			fmt.Println("📋 SUMMARY MODE - Showing directory-level overview")
		}
	}
	fmt.Println()

	err := copyFiles(sourceDir, destDir, dryRun, verbose)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}

// validateDirectory checks if a directory exists and is accessible
func validateDirectory(dir, label string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s directory does not exist: %s", label, dir)
		}
		return fmt.Errorf("Cannot access %s directory: %s (%v)", label, dir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s path is not a directory: %s", label, dir)
	}

	return nil
}

// dirStats holds statistics for a directory
type dirStats struct {
	path         string
	fileCount    int
	skippedCount int
	totalBytes   int64
}

// copyFiles walks through the source directory and copies files that don't exist in destination
func copyFiles(sourceDir, destDir string, dryRun, verbose bool) error {
	var filesCopied, filesSkipped int
	var totalBytes int64

	// For summary mode (default in dry-run), track stats by directory
	// Summary mode is enabled when dryRun is true and verbose is false
	summary := dryRun && !verbose
	dirStatsMap := make(map[string]*dirStats)

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("⚠️  Warning: Cannot access %s: %v\n", path, err)
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Calculate relative path from source
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			fmt.Printf("⚠️  Warning: Cannot determine relative path for %s: %v\n", path, err)
			return nil
		}

		// Determine destination path
		destPath := filepath.Join(destDir, relPath)

		// Get the directory for this file (for summary mode)
		relDir := filepath.Dir(relPath)
		if relDir == "." {
			relDir = "(root)"
		}

		// Check if file already exists in destination
		if _, err := os.Stat(destPath); err == nil {
			if !summary {
				fmt.Printf("⏭️  Skipping (already exists): %s\n", relPath)
			}
			filesSkipped++

			// Track in dir stats
			if summary && dryRun {
				if _, exists := dirStatsMap[relDir]; !exists {
					dirStatsMap[relDir] = &dirStats{path: relDir}
				}
				dirStatsMap[relDir].skippedCount++
			}
			return nil
		}

		// File doesn't exist in destination, copy it
		if dryRun {
			if summary {
				// Track stats for summary
				if _, exists := dirStatsMap[relDir]; !exists {
					dirStatsMap[relDir] = &dirStats{path: relDir}
				}
				dirStatsMap[relDir].fileCount++
				dirStatsMap[relDir].totalBytes += info.Size()
			} else {
				fmt.Printf("🔍 [DRY RUN] Would copy: %s (%.2f KB)\n", relPath, float64(info.Size())/1024)
			}
		} else {
			if err := copyFile(path, destPath); err != nil {
				fmt.Printf("❌ Failed to copy %s: %v\n", relPath, err)
				return nil // Continue with other files
			}
			fmt.Printf("✅ Copied: %s (%.2f KB)\n", relPath, float64(info.Size())/1024)
		}

		filesCopied++
		totalBytes += info.Size()
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error walking source directory: %v", err)
	}

	// Print directory-level summary if requested
	if dryRun && summary && len(dirStatsMap) > 0 {
		fmt.Printf("📋 Directory Summary (generated at %s):\n\n", time.Now().Format("2006-01-02 15:04:05"))

		// Sort directories for consistent output
		dirs := make([]string, 0, len(dirStatsMap))
		for dir := range dirStatsMap {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)

		for _, dir := range dirs {
			stats := dirStatsMap[dir]
			// Only show directories that have files to copy (skip directories with only existing files)
			if stats.fileCount > 0 {
				fmt.Printf("📂 %s\n", stats.path)
				fmt.Printf("   ✅ Would copy: %d files (%.2f MB)\n", stats.fileCount, float64(stats.totalBytes)/(1024*1024))
				fmt.Println()
			}
		}
	}

	// Print summary
	fmt.Println("📊 Overall Summary:")
	if dryRun {
		fmt.Printf("   Files that would be copied: %d\n", filesCopied)
	} else {
		fmt.Printf("   Files copied: %d\n", filesCopied)
	}
	fmt.Printf("   Files skipped (already exist): %d\n", filesSkipped)
	fmt.Printf("   Total size: %.2f MB\n", float64(totalBytes)/(1024*1024))

	return nil
}

// copyFile copies a single file from src to dst, creating destination directories as needed
func copyFile(src, dst string) error {
	// Create destination directory if it doesn't exist
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dstDir, err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer dstFile.Close()

	// Copy file contents
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Copy file permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %v", err)
	}

	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to set file permissions: %v", err)
	}

	return nil
}