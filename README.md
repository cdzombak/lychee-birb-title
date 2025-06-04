# Lychee BB Tag

This program processes photos and videos from a Lychee photo album, performing OCR on the bottom 20% of each image (or the first frame of each video) and updating the photo titles in the database.

## Prerequisites

- Go 1.21 or later
- MySQL database
- Google Cloud account with Vision API enabled
- Google Cloud credentials file
- ffmpeg (for video processing)

## Installation

1. Copy `config.json` and update it with your settings:
   ```json
   {
       "database": {
           "host": "localhost",
           "port": 3306,
           "user": "your_username",
           "password": "your_password",
           "database": "your_database"
       },
       "google_cloud": {
           "project_id": "your_project_id",
           "credentials_file": "path/to/credentials.json"
       },
       "base_url": "https://pictures.dzombak.com/uploads/",
       "album_id": "FHaZFQEiAVAvrEbhkQo_CrBB"
   }
   ```

2. Place your Google Cloud credentials file at the path specified in the config.

## Building

```bash
go build
```

## Usage

By default, the program runs in dry-run mode, which means it will process all images and videos but won't update the database. It will log the OCR results and file URLs for manual verification.

```bash
./lychee-bb-tag
```

To actually update the database with the OCR results:

```bash
./lychee-bb-tag -dry-run=false
```

To create Things tasks for photos that have no text detected:

```bash
./lychee-bb-tag -things
```

## How it Works

1. Connects to the MySQL database
2. Queries for photos in the specified album where the title is a UUID
3. For each photo/video:
   - Downloads the file
   - If it's a video, extracts the first frame
   - Crops the bottom 20% of the image
   - Performs OCR using Google Cloud Vision API
   - If no text is detected and --things flag is set, creates a Things task for manual review
   - If OCR confidence is high enough (>0.8) and not in dry-run mode, updates the photo title in the database
4. Logs all operations and results

## Supported File Types

- Images: JPG, JPEG, PNG, GIF, BMP, WebP
- Videos: MP4, MOV, AVI
