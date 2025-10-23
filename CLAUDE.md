# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`sync-disc` is a simple Go application for syncing files between two directories (typically external drives). It copies files from a source directory to a destination directory, preserving directory structure and skipping files that already exist in the destination.

## Development Commands

### Build
```bash
go build -o sync-disc
```

### Run
```bash
# Show help
./sync-disc

# Dry run to preview what would be copied
./sync-disc -source /path/to/source -dest /path/to/destination -dry-run

# Actual copy operation
./sync-disc -source /path/to/source -dest /path/to/destination
```

## Architecture

The application is a single-file Go program (`main.go`) with the following key functions:

- **`validateDirectory(dir, label string) error`**: Validates that a directory exists and is accessible
- **`copyFiles(sourceDir, destDir string, dryRun bool) error`**: Main file walking logic using `filepath.Walk` to traverse source directory
- **`copyFile(src, dst string) error`**: Copies a single file with permission preservation

### Implementation Details

- **Command-line parsing**: Uses Go's `flag` package for `-source`, `-dest`, and `-dry-run` flags
- **Directory creation**: Uses `os.MkdirAll` to create nested destination directories as needed
- **File comparison**: Checks if files already exist in destination using `os.Stat`
- **File copying**: Uses `io.Copy` for efficient file copying and `os.Chmod` to preserve permissions
- **Error handling**: Continues processing remaining files even when individual file operations fail
- **Progress reporting**: Shows real-time progress with emojis and file sizes in KB/MB

### Key Features

- **Dry-run mode**: Preview what would be copied without actually copying
- **Directory structure preservation**: Maintains relative paths from source to destination
- **Duplicate detection**: Skips files that already exist in destination
- **Error handling**: Continues operation even if individual files fail
- **Progress feedback**: Shows copied/skipped files with sizes and summary statistics

## Use Cases

Primarily designed for:
- Backing up photos/videos to external drives
- Syncing content between drives where one is more up-to-date
- Merging collections while avoiding duplicates