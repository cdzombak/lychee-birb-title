# Lychee BB Tagger

This program processes untitled photos and videos from a Lychee photo album, performing OCR on the bottom 20% of each image (or the first frame of each video) and updating the photo titles in the database.

## Requirements

- Go (1.21 or later)
- Access to the Lychee MySQL database
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

## Usage

By default, the program runs in dry-run mode, which means it will process all images and videos but won't update the database. It will log the OCR results and file URLs for manual verification.

```bash
go run .
```

To actually update the database with the OCR results:

```bash
go run . -dry-run=false
```

To create Things tasks for photos that have no text detected:

```bash
go run . -things=true
```

## Supported File Types

- Images: JPG, JPEG, PNG, GIF, BMP, WebP
- Videos: MP4, MOV, AVI

## Author & LICENSE

- Chris Dzombak
- This is a private repo; internal use only.
