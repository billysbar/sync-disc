# sync-disc

A simple, efficient Go utility for syncing files between two directories, perfect for backing up photos, videos, and other files to external drives.

## Features

- **Duplicate Detection**: Automatically skips files that already exist in the destination
- **Directory Structure Preservation**: Maintains the exact folder hierarchy from source to destination
- **Dry-Run Mode**: Preview what would be copied without actually copying anything
- **Summary Mode**: Get a high-level directory overview instead of individual file details
- **Progress Feedback**: Real-time status with file sizes and emoji indicators
- **Permission Preservation**: Copies file permissions from source to destination
- **Error Resilience**: Continues operation even if individual file operations fail

## Installation

### Prerequisites

- Go 1.16 or higher

### Build from Source

```bash
git clone <repository-url>
cd sync-disc
go build -o sync-disc
```

This creates the `sync-disc` executable in the current directory.

## Usage

### Basic Syntax

```bash
./sync-disc -source /path/to/source -dest /path/to/destination [OPTIONS]
```

### Options

| Flag | Description |
|------|-------------|
| `-source` | Source directory to copy files from (required) |
| `-dest` | Destination directory to copy files to (required) |
| `-dry-run` | Preview what would be copied without actually copying |
| `-summary` | Show directory-level summary instead of individual files (use with `-dry-run`) |

### Examples

#### Show Help

```bash
./sync-disc
```

#### Preview Changes (Dry Run)

See what would be copied without actually copying:

```bash
./sync-disc -source /Volumes/Camera/DCIM -dest /Volumes/Backup/Photos -dry-run
```

#### Preview with Directory Summary

Get a high-level overview by directory:

```bash
./sync-disc -source /Volumes/Camera/DCIM -dest /Volumes/Backup/Photos -dry-run -summary
```

#### Actual Copy Operation

Perform the actual file sync:

```bash
./sync-disc -source /Volumes/Camera/DCIM -dest /Volumes/Backup/Photos
```

## How It Works

1. **Validation**: Checks that both source and destination directories exist and are accessible
2. **Directory Walking**: Recursively traverses the source directory using `filepath.Walk`
3. **Duplicate Check**: For each file, checks if it already exists in the destination
4. **Selective Copying**: Only copies files that don't exist in the destination
5. **Structure Creation**: Automatically creates nested directories in the destination as needed
6. **Progress Reporting**: Shows real-time status for each file processed

### File Comparison Logic

Files are considered duplicates if a file with the same relative path exists in the destination directory. The tool uses simple existence checking (`os.Stat`) rather than comparing file contents or modification times.

## Output Examples

### Standard Mode

```
📁 Source: /Volumes/Camera/DCIM
📁 Destination: /Volumes/Backup/Photos

✅ Copied: 2024/IMG_001.jpg (3.45 MB)
✅ Copied: 2024/IMG_002.jpg (2.87 MB)
⏭️  Skipping (already exists): 2024/IMG_003.jpg
✅ Copied: 2024/Videos/VID_001.mp4 (125.34 MB)

📊 Overall Summary:
   Files copied: 3
   Files skipped (already exist): 1
   Total size: 131.66 MB
```

### Dry-Run with Summary Mode

```
📁 Source: /Volumes/Camera/DCIM
📁 Destination: /Volumes/Backup/Photos
🔍 DRY RUN MODE - No files will be copied
📋 SUMMARY MODE - Showing directory-level overview

📋 Directory Summary (generated at 2025-10-23 14:30:45):

📂 2024/January
   ✅ Would copy: 15 files (45.23 MB)
   ⏭️  Would skip: 3 files (already exist)

📂 2024/February
   ✅ Would copy: 8 files (28.91 MB)

📂 2024/Videos
   ✅ Would copy: 2 files (250.67 MB)
   ⏭️  Would skip: 1 files (already exist)

📊 Overall Summary:
   Files that would be copied: 25
   Files skipped (already exist): 4
   Total size: 324.81 MB
```

## Use Cases

### Photo/Video Backup

Ideal for backing up camera memory cards to external drives:

```bash
./sync-disc -source /Volumes/SD_CARD/DCIM -dest /Volumes/Backup/Photos/2025
```

### Drive Synchronization

Sync content between two drives where one is more up-to-date:

```bash
./sync-disc -source /Volumes/WorkDrive/Projects -dest /Volumes/BackupDrive/Projects
```

### Collection Merging

Merge photo/video collections while automatically avoiding duplicates:

```bash
./sync-disc -source /Volumes/OldDrive/Photos -dest /Volumes/MainDrive/PhotoLibrary
```

## Technical Details

### Architecture

The application is a single-file Go program (`main.go`) with the following key components:

- **`main()`**: Entry point that handles command-line parsing and orchestrates the sync operation
- **`validateDirectory()`**: Validates that directories exist and are accessible
- **`copyFiles()`**: Main file walking logic that traverses the source directory
- **`copyFile()`**: Copies a single file with permission preservation

### Implementation Highlights

- **Standard Library Only**: No external dependencies required
- **Efficient File Copying**: Uses `io.Copy` for optimal performance
- **Path Handling**: Uses `filepath.Walk` and `filepath.Rel` for cross-platform path handling
- **Error Handling**: Individual file failures don't stop the entire operation
- **Memory Efficient**: Processes files one at a time without loading entire contents into memory

### Limitations

- **No Content Comparison**: Files are compared by path only, not by content or modification time
- **One-Way Sync**: Only copies from source to destination (not bidirectional)
- **No Deletion**: Never deletes files from the destination
- **No Modification Updates**: Doesn't update files that have changed since the last sync
- **No Progress Bar**: Shows per-file status but no overall progress percentage

## Error Handling

The tool is designed to be resilient:

- Individual file access errors generate warnings but don't stop the operation
- Directory permission issues are reported and skipped
- Failed file copies are logged but processing continues with remaining files
- Invalid source/destination paths cause immediate exit with clear error messages

## Performance Considerations

- **Large Files**: Handles files of any size efficiently using streaming I/O
- **Many Files**: Processes files sequentially; performance scales linearly with file count
- **Network Drives**: Works with network-mounted drives but performance depends on network speed
- **No Parallel Processing**: Files are copied one at a time (no concurrent operations)

## License

[Add your license information here]

## Contributing

[Add contribution guidelines here]

## Author

[Add author information here]

## Version History

- **Initial Release**: Basic sync functionality with dry-run and summary modes
