package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

func main() {
	var sourceDir, destDir string
	var dryRun, verbose bool
	var workers int

	flag.StringVar(&sourceDir, "source", "", "Source directory to copy files from")
	flag.StringVar(&destDir, "dest", "", "Destination directory to copy files to")
	flag.BoolVar(&dryRun, "dry-run", false, "Perform a dry run - show what would be copied without actually copying")
	flag.BoolVar(&verbose, "verbose", false, "Show individual files instead of directory-level summary (use with -dry-run)")
	flag.IntVar(&workers, "workers", 4, "Number of concurrent workers (1-16, default: 4)")
	flag.Parse()

	if sourceDir == "" || destDir == "" {
		fmt.Printf("Usage: %s -source /path/to/source -dest /path/to/destination [-dry-run] [-verbose] [-workers N]\n", os.Args[0])
		fmt.Println("\nThis tool copies all files from source to destination, skipping files that already exist in destination.")
		fmt.Println("Directory structure is preserved.")
		fmt.Println("\nOptions:")
		fmt.Println("  -dry-run    Show what would be copied without actually copying (shows summary by default)")
		fmt.Println("  -verbose    Show individual files instead of directory-level summary (use with -dry-run)")
		fmt.Println("  -workers N  Number of concurrent workers (1-16, default: 4)")
		os.Exit(1)
	}

	// Validate worker count
	if workers < 1 {
		workers = 1
	} else if workers > 16 {
		workers = 16
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
	fmt.Printf("👷 Workers: %d\n", workers)
	if dryRun {
		fmt.Println("🔍 DRY RUN MODE - No files will be copied")
		if !verbose {
			fmt.Println("📋 SUMMARY MODE - Showing directory-level overview")
		}
	}
	fmt.Println()

	err := copyFiles(sourceDir, destDir, dryRun, verbose, workers)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Clean up small files in destination (only during actual sync, not dry-run)
	if !dryRun {
		err = cleanupSmallFiles(destDir)
		if err != nil {
			fmt.Printf("❌ Cleanup error: %v\n", err)
			os.Exit(1)
		}
	}

	// Beep to indicate completion
	fmt.Print("\a")
	time.Sleep(200 * time.Millisecond)
	fmt.Print("\a")
	time.Sleep(200 * time.Millisecond)
	fmt.Print("\a")
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

// FileLockManager manages file locks to prevent race conditions
type FileLockManager struct {
	locks map[string]*sync.RWMutex
	mu    sync.Mutex
}

// NewFileLockManager creates a new file lock manager
func NewFileLockManager() *FileLockManager {
	return &FileLockManager{
		locks: make(map[string]*sync.RWMutex),
	}
}

// LockFile acquires a lock for a specific file path
func (flm *FileLockManager) LockFile(path string) {
	normalPath := filepath.Clean(path)
	flm.mu.Lock()
	if _, exists := flm.locks[normalPath]; !exists {
		flm.locks[normalPath] = &sync.RWMutex{}
	}
	lock := flm.locks[normalPath]
	flm.mu.Unlock()
	lock.Lock()
}

// UnlockFile releases a lock for a specific file path
func (flm *FileLockManager) UnlockFile(path string) {
	flm.mu.Lock()
	normalPath := filepath.Clean(path)
	if lock, exists := flm.locks[normalPath]; exists {
		flm.mu.Unlock()
		lock.Unlock()
		return
	}
	flm.mu.Unlock()
}

// Global file lock manager
var globalFileLockManager = NewFileLockManager()

// CopyJob represents a single file copy job
type CopyJob struct {
	SourcePath string
	DestPath   string
	RelPath    string
	Size       int64
}

// CopyResult represents the result of a copy operation
type CopyResult struct {
	Job     CopyJob
	Success bool
	Skipped bool
	Error   error
}

// ProgressTracker tracks copy progress with thread safety
type ProgressTracker struct {
	Total     int
	Copied    int
	Skipped   int
	Failed    int
	TotalSize int64
	StartTime time.Time
	mu        sync.Mutex
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{
		Total:     total,
		StartTime: time.Now(),
	}
}

// UpdateCopied increments the copied count
func (pt *ProgressTracker) UpdateCopied(size int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.Copied++
	pt.TotalSize += size
}

// UpdateSkipped increments the skipped count
func (pt *ProgressTracker) UpdateSkipped() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.Skipped++
}

// UpdateFailed increments the failed count
func (pt *ProgressTracker) UpdateFailed() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.Failed++
}

// GetStats returns current progress statistics
func (pt *ProgressTracker) GetStats() (copied, skipped, failed int, totalSize int64, elapsed time.Duration) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.Copied, pt.Skipped, pt.Failed, pt.TotalSize, time.Since(pt.StartTime)
}

// dirStats holds statistics for a directory
type dirStats struct {
	path         string
	fileCount    int
	skippedCount int
	totalBytes   int64
}

// copyFiles walks through the source directory and copies files that don't exist in destination
func copyFiles(sourceDir, destDir string, dryRun, verbose bool, workers int) error {
	// First, collect all files to copy
	fmt.Println("📂 Scanning source directory...")
	var jobs []CopyJob
	summary := dryRun && !verbose

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

		// Check if file already exists in destination - skip it
		if _, err := os.Stat(destPath); err == nil {
			return nil // File exists, will be counted as skipped later
		}

		// File doesn't exist in destination, add to jobs
		jobs = append(jobs, CopyJob{
			SourcePath: path,
			DestPath:   destPath,
			RelPath:    relPath,
			Size:       info.Size(),
		})

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error walking source directory: %v", err)
	}

	if len(jobs) == 0 {
		fmt.Println("✅ No files need to be copied (all files already exist in destination)")
		return nil
	}

	fmt.Printf("📝 Found %d files to copy\n", len(jobs))
	if !dryRun {
		fmt.Printf("🚀 Starting %d workers...\n\n", workers)
	} else {
		fmt.Println()
	}

	// Process jobs with worker pool
	results := processCopyJobsConcurrently(jobs, workers, dryRun, verbose, summary)

	// Print summary
	filesCopied := 0
	filesSkipped := 0
	filesFailed := 0
	var totalBytes int64
	dirStatsMap := make(map[string]*dirStats)

	for _, result := range results {
		if result.Skipped {
			filesSkipped++
		} else if result.Success {
			filesCopied++
			totalBytes += result.Job.Size

			// Track for summary mode
			if summary {
				relDir := filepath.Dir(result.Job.RelPath)
				if relDir == "." {
					relDir = "(root)"
				}
				if _, exists := dirStatsMap[relDir]; !exists {
					dirStatsMap[relDir] = &dirStats{path: relDir}
				}
				dirStatsMap[relDir].fileCount++
				dirStatsMap[relDir].totalBytes += result.Job.Size
			}
		} else {
			filesFailed++
		}
	}

	// Print directory-level summary if requested
	if dryRun && summary && len(dirStatsMap) > 0 {
		fmt.Printf("\n📋 Directory Summary:\n\n")

		// Sort directories for consistent output
		dirs := make([]string, 0, len(dirStatsMap))
		for dir := range dirStatsMap {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)

		for _, dir := range dirs {
			stats := dirStatsMap[dir]
			if stats.fileCount > 0 {
				fmt.Printf("📂 %s\n", stats.path)
				fmt.Printf("   ✅ Would copy: %d files (%.2f MB)\n", stats.fileCount, float64(stats.totalBytes)/(1024*1024))
				fmt.Println()
			}
		}
	}

	// Print overall summary
	fmt.Println("📊 Overall Summary:")
	if dryRun {
		fmt.Printf("   Files that would be copied: %d\n", filesCopied)
	} else {
		fmt.Printf("   Files copied: %d\n", filesCopied)
	}
	fmt.Printf("   Files skipped (already exist): %d\n", filesSkipped)
	if filesFailed > 0 {
		fmt.Printf("   Files failed: %d\n", filesFailed)
	}
	fmt.Printf("   Total size: %.2f MB\n", float64(totalBytes)/(1024*1024))

	return nil
}

// processCopyJobsConcurrently processes copy jobs using a worker pool
func processCopyJobsConcurrently(jobs []CopyJob, numWorkers int, dryRun, verbose, summary bool) []CopyResult {
	// Create channels
	jobChan := make(chan CopyJob, len(jobs))
	resultChan := make(chan CopyResult, len(jobs))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create progress tracker
	progress := NewProgressTracker(len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go copyWorker(i, jobChan, resultChan, ctx, &wg, dryRun, verbose, summary, progress)
	}

	// Send jobs
	go func() {
		for _, job := range jobs {
			select {
			case jobChan <- job:
			case <-ctx.Done():
				return
			}
		}
		close(jobChan)
	}()

	// Start progress reporter (only if not in verbose mode)
	var progressWg sync.WaitGroup
	if !verbose && !dryRun {
		progressWg.Add(1)
		go progressReporter(progress, &progressWg, ctx)
	}

	// Collect results
	var results []CopyResult
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		results = append(results, result)
	}

	// Stop progress reporter
	cancel()
	progressWg.Wait()

	// Print final progress if we were showing progress
	if !verbose && !dryRun {
		copied, skipped, failed, totalSize, elapsed := progress.GetStats()
		fmt.Printf("\r✅ Completed: %d copied, %d skipped, %d failed | %.2f MB | %v\n\n",
			copied, skipped, failed, float64(totalSize)/(1024*1024), elapsed.Round(time.Second))
	}

	return results
}

// copyWorker processes copy jobs from the job channel
func copyWorker(id int, jobs <-chan CopyJob, results chan<- CopyResult, ctx context.Context, wg *sync.WaitGroup, dryRun, verbose, summary bool, progress *ProgressTracker) {
	defer wg.Done()

	for {
		select {
		case job, ok := <-jobs:
			if !ok {
				return
			}

			result := CopyResult{
				Job:     job,
				Success: false,
				Skipped: false,
			}

			// Check if file still doesn't exist (double-check for race conditions)
			if _, err := os.Stat(job.DestPath); err == nil {
				result.Skipped = true
				if verbose && !summary {
					fmt.Printf("⏭️  Skipping (already exists): %s\n", job.RelPath)
				}
				progress.UpdateSkipped()
				results <- result
				continue
			}

			if dryRun {
				// Dry run mode - just report what would happen
				if verbose {
					fmt.Printf("🔍 [DRY RUN] Would copy: %s (%.2f KB)\n", job.RelPath, float64(job.Size)/1024)
				}
				result.Success = true
				progress.UpdateCopied(job.Size)
			} else {
				// Actual copy with file locking
				err := copyFileWithLock(job.SourcePath, job.DestPath)
				if err != nil {
					result.Error = err
					progress.UpdateFailed()
					if verbose {
						fmt.Printf("❌ Failed to copy %s: %v\n", job.RelPath, err)
					}
				} else {
					result.Success = true
					progress.UpdateCopied(job.Size)
					if verbose {
						fmt.Printf("✅ Copied: %s (%.2f KB)\n", job.RelPath, float64(job.Size)/1024)
					}
				}
			}

			results <- result

		case <-ctx.Done():
			return
		}
	}
}

// progressReporter displays progress updates
func progressReporter(progress *ProgressTracker, wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			copied, skipped, failed, totalSize, elapsed := progress.GetStats()
			fmt.Printf("\r⏳ Progress: %d copied, %d skipped, %d failed | %.2f MB | %v",
				copied, skipped, failed, float64(totalSize)/(1024*1024), elapsed.Round(time.Second))
		case <-ctx.Done():
			return
		}
	}
}

// copyFileWithLock copies a file with file locking to prevent race conditions
func copyFileWithLock(src, dst string) error {
	// Lock both source and destination paths
	globalFileLockManager.LockFile(src)
	defer globalFileLockManager.UnlockFile(src)

	globalFileLockManager.LockFile(dst)
	defer globalFileLockManager.UnlockFile(dst)

	return copyFile(src, dst)
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

// cleanupSmallFiles walks through the destination directory and deletes files smaller than 10KB
func cleanupSmallFiles(destDir string) error {
	const minSize = 10240 // 10KB in bytes
	var filesDeleted int

	err := filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Delete files smaller than 10KB
		if info.Size() < minSize {
			if err := os.Remove(path); err != nil {
				// Continue even if deletion fails
				return nil
			}
			filesDeleted++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("Error walking destination directory: %v", err)
	}

	// Print summary
	if filesDeleted > 0 {
		fmt.Printf("\n🧹 Cleanup: Deleted %d files under 10KB\n", filesDeleted)
	}

	return nil
}