![Lychee Birb Title](banner.jpg)

# Lychee Birb Title

This program processes photos and videos from a [Lychee](https://github.com/LycheeOrg/Lychee) photo album whose titles are UUIDs, performing OCR on the bottom 20% of each image (or the first frame of each video) and updating the photo titles in the database.

This is intended to provide correct titles on [Bird Buddy](https://mybirdbuddy.com) photos uploaded from an iPhone; see [my Bird Buddy album](https://pictures.dzombak.com/gallery/FHaZFQEiAVAvrEbhkQo_CrBB) for an example.

For any photos without text, the program creates tasks in the [Things](https://culturedcode.com/things/) todo app for manual review.

## Requirements

- Go (1.21 or later)
- Access to a Lychee database (MySQL, PostgreSQL, or SQLite)
- Google Cloud account with Vision API enabled
- ffmpeg (for video processing)

## Configuration

1. Copy the appropriate sample configuration file to `config.json` and update it with your settings:
   - For MySQL: `config.sample.json` (default)
   - For PostgreSQL: `config.postgres.sample.json`
   - For SQLite: `config.sqlite.sample.json`
2. Place your Google Cloud credentials file at the path specified in the config.

### Database Configuration

The program supports three database types:

**MySQL:**
```json
{
    "database": {
        "type": "mysql",
        "host": "localhost",
        "port": 3306,
        "user": "lychee_user",
        "password": "your_password",
        "database": "lychee"
    }
}
```

**PostgreSQL:**
```json
{
    "database": {
        "type": "postgres",
        "host": "localhost",
        "port": 5432,
        "user": "lychee_user",
        "password": "your_password",
        "database": "lychee"
    }
}
```

**SQLite:**
```json
{
    "database": {
        "type": "sqlite",
        "host": "",
        "port": 0,
        "user": "",
        "password": "",
        "database": "/path/to/lychee.db"
    }
}
```

For SQLite, only the `type` and `database` fields are required. The `database` field should contain the full path to your SQLite database file.

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

## Author & License

- [Chris Dzombak](https://github.com/cdzombak)
- Licensed under the MIT License. See [LICENSE](LICENSE) for details.
