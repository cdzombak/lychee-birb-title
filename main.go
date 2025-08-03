package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	vision "cloud.google.com/go/vision/apiv1"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/option"
)

var Version = "<dev>"

type Config struct {
	Database struct {
		Type     string `json:"type"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Database string `json:"database"`
	} `json:"database"`
	GoogleCloud struct {
		ProjectID       string `json:"project_id"`
		CredentialsFile string `json:"credentials_file"`
	} `json:"gcp"`
	BaseURL   string `json:"base_url"`
	AlbumID   string `json:"album_id"`
	StateFile string `json:"statefile"`
}

type Photo struct {
	ID       string
	Title    string
	ImageURL string
}

type PhotoError struct {
	ID      string
	URL     string
	Error   string
	WebLink string
}

type State struct {
	NoTextPhotos map[string]bool `json:"no_text_photos"`
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %v", err)
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, fmt.Errorf("error decoding config file: %v", err)
	}

	return &config, nil
}

func isUUID(title string) bool {
	// Strip common image and video extensions (case insensitive)
	extensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".mp4", ".mov", ".avi"}
	title = strings.ToLower(title)
	for _, ext := range extensions {
		title = strings.TrimSuffix(title, ext)
	}

	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	return uuidPattern.MatchString(title)
}

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp4" || ext == ".mov" || ext == ".avi"
}

func extractFirstFrame(videoPath string) (string, error) {
	// Create a temporary file for the output frame
	tmpFile, err := os.CreateTemp("", "frame-*.jpg")
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %v", err)
	}
	defer tmpFile.Close()

	// Use ffmpeg to extract the first frame with specific quality settings
	cmd := exec.Command("ffmpeg",
		"-i", videoPath, // Input video
		"-vframes", "1", // Extract only one frame
		"-q:v", "2", // High quality
		"-y",           // Overwrite output file if it exists
		tmpFile.Name()) // Output file

	// Capture both stdout and stderr for better error reporting
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error extracting frame: %v (ffmpeg output: %s)", err, string(output))
	}

	return tmpFile.Name(), nil
}

func downloadFile(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("error downloading file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	// Determine file extension from URL
	ext := filepath.Ext(url)
	if ext == "" {
		ext = ".jpg" // Default to jpg if no extension found
	}

	// Create a temporary file with the appropriate extension
	tmpFile, err := os.CreateTemp("", "file-*"+ext)
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %v", err)
	}
	defer tmpFile.Close()

	// Copy the file data
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", fmt.Errorf("error saving file: %v", err)
	}

	return tmpFile.Name(), nil
}

func cropImage(inputPath string) (string, error) {
	// Open the input image
	file, err := os.Open(inputPath)
	if err != nil {
		return "", fmt.Errorf("error opening image: %v", err)
	}
	defer file.Close()

	// Decode the image
	img, err := jpeg.Decode(file)
	if err != nil {
		return "", fmt.Errorf("error decoding image: %v", err)
	}

	// Get image bounds
	bounds := img.Bounds()
	height := bounds.Dy()

	// Calculate crop dimensions (bottom 20%)
	cropHeight := height / 5
	cropY := height - cropHeight

	// Create a new image for the cropped portion
	cropped := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), cropHeight))

	// Copy the bottom 20% of the image
	for y := 0; y < cropHeight; y++ {
		for x := 0; x < bounds.Dx(); x++ {
			cropped.Set(x, y, img.At(x, cropY+y))
		}
	}

	// Create output file
	outputPath := inputPath + ".cropped.jpg"
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()

	// Encode the cropped image
	if err := jpeg.Encode(outFile, cropped, nil); err != nil {
		return "", fmt.Errorf("error encoding cropped image: %v", err)
	}

	return outputPath, nil
}

func performOCR(ctx context.Context, imagePath string, client *vision.ImageAnnotatorClient) (string, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("error opening image: %v", err)
	}
	defer file.Close()

	image, err := vision.NewImageFromReader(file)
	if err != nil {
		return "", fmt.Errorf("error creating vision image: %v", err)
	}

	annotations, err := client.DetectTexts(ctx, image, nil, 1)
	if err != nil {
		return "", fmt.Errorf("error detecting text: %v", err)
	}

	if len(annotations) == 0 {
		return "", fmt.Errorf("no text detected")
	}

	// Get the first (and should be only) text annotation
	return annotations[0].Description, nil
}

func loadState(path string) (*State, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state if file doesn't exist
			return &State{NoTextPhotos: make(map[string]bool)}, nil
		}
		return nil, fmt.Errorf("error opening state file: %v", err)
	}
	defer file.Close()

	var state State
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return nil, fmt.Errorf("error decoding state file: %v", err)
	}

	return &state, nil
}

func saveState(path string, state *State) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating state file: %v", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(state); err != nil {
		return fmt.Errorf("error encoding state file: %v", err)
	}

	return nil
}

func buildConnectionString(config *Config) (string, string, error) {
	switch strings.ToLower(config.Database.Type) {
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			config.Database.User,
			config.Database.Password,
			config.Database.Host,
			config.Database.Port,
			config.Database.Database,
		)
		return "mysql", dsn, nil
	case "postgres", "postgresql":
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			config.Database.Host,
			config.Database.Port,
			config.Database.User,
			config.Database.Password,
			config.Database.Database,
		)
		return "postgres", dsn, nil
	case "sqlite", "sqlite3":
		return "sqlite3", config.Database.Database, nil
	default:
		return "", "", fmt.Errorf("unsupported database type: %s", config.Database.Type)
	}
}

func main() {
	dryRun := flag.Bool("dry-run", true, "Perform a dry run without updating the database")
	showVersion := flag.Bool("version", false, "Show version and exit")
	configFile := flag.String("config", "config.json", "Path to configuration file")
	maxImages := flag.Int("max", 0, "Maximum number of images to process (0 for unlimited)")
	things := flag.Bool("things", false, "Create Things tasks for photos with no text detected")
	flag.Parse()

	if *showVersion {
		fmt.Printf("lychee-birb-title version %s\n", Version)
		os.Exit(0)
	}

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Load state
	state, err := loadState(config.StateFile)
	if err != nil {
		log.Fatalf("Error loading state: %v", err)
	}

	// Initialize database connection
	driver, dsn, err := buildConnectionString(config)
	if err != nil {
		log.Fatalf("Error building connection string: %v", err)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	// Initialize Google Cloud Vision client
	ctx := context.Background()
	client, err := vision.NewImageAnnotatorClient(ctx,
		option.WithCredentialsFile(config.GoogleCloud.CredentialsFile))
	if err != nil {
		log.Fatalf("Error creating Vision client: %v", err)
	}
	defer client.Close()

	// Query for photos
	query := `
		SELECT p.id, p.title, sv.short_path
		FROM photos p
		JOIN size_variants sv ON p.id = sv.photo_id
		JOIN photo_album pa on p.id = pa.photo_id
		WHERE pa.album_id = ? AND sv.type = 0
	`

	rows, err := db.Query(query, config.AlbumID)
	if err != nil {
		log.Fatalf("Error querying photos: %v", err)
	}
	defer rows.Close()

	photoCount := 0
	processedCount := 0
	updatedCount := 0
	thingsCount := 0
	var errors []PhotoError

	for rows.Next() {
		var photo Photo
		var shortPath string
		if err := rows.Scan(&photo.ID, &photo.Title, &shortPath); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		// Skip if title is not a UUID
		if !isUUID(photo.Title) {
			continue
		}

		// Skip if we've already processed this photo and found no text
		if state.NoTextPhotos[photo.ID] {
			log.Printf("Skipping photo %s (previously found no text)", photo.ID)
			continue
		}

		// Check if we've reached the maximum number of images to process
		if *maxImages > 0 && photoCount >= *maxImages {
			log.Printf("Reached maximum number of images to process (%d)", *maxImages)
			break
		}

		photoCount++

		// Clean up the base URL and paths
		baseURL := strings.TrimRight(config.BaseURL, "/")
		shortPath = strings.TrimLeft(shortPath, "/")
		photo.ImageURL = fmt.Sprintf("%s/uploads/%s", baseURL, shortPath)
		webLink := fmt.Sprintf("%s/gallery/%s/%s", baseURL, config.AlbumID, photo.ID)

		// Download and process the file
		filePath, err := downloadFile(photo.ImageURL)
		if err != nil {
			errors = append(errors, PhotoError{
				ID:      photo.ID,
				URL:     photo.ImageURL,
				Error:   fmt.Sprintf("Error downloading file: %v", err),
				WebLink: webLink,
			})
			continue
		}
		defer func() { _ = os.Remove(filePath) }()

		// If it's a video, extract the first frame
		var imagePath string
		if isVideoFile(photo.ImageURL) {
			imagePath, err = extractFirstFrame(filePath)
			if err != nil {
				errors = append(errors, PhotoError{
					ID:      photo.ID,
					URL:     photo.ImageURL,
					Error:   fmt.Sprintf("Error extracting frame from video: %v", err),
					WebLink: webLink,
				})
				continue
			}
			defer func() { _ = os.Remove(imagePath) }()
		} else {
			imagePath = filePath
		}

		// Now crop the image (or the extracted frame)
		croppedPath, err := cropImage(imagePath)
		if err != nil {
			errors = append(errors, PhotoError{
				ID:      photo.ID,
				URL:     photo.ImageURL,
				Error:   fmt.Sprintf("Error cropping image: %v", err),
				WebLink: webLink,
			})
			continue
		}
		defer func() { _ = os.Remove(croppedPath) }()

		processedCount++

		text, err := performOCR(ctx, croppedPath, client)
		if err != nil {
			if strings.Contains(err.Error(), "no text detected") {
				// If no text detected and --things flag is set, create a task for manual review
				if *things {
					// Add to state file
					state.NoTextPhotos[photo.ID] = true
					if err := saveState(config.StateFile, state); err != nil {
						log.Printf("Error saving state: %v", err)
					}

					// Create Things URL for manual review
					thingsURL := fmt.Sprintf("things:///add?title=%s&notes=%s",
						url.PathEscape(fmt.Sprintf("[Lychee BB] Review %s", photo.ID)),
						url.PathEscape(fmt.Sprintf("Image: %s\nWeb UI: %s", photo.ImageURL, webLink)))
					if *dryRun {
						fmt.Printf("Would open Things URL: %s\n", thingsURL)
					} else {
						if err := exec.Command("open", thingsURL).Run(); err != nil {
							log.Printf("Error opening Things URL: %v", err)
						}
					}
					thingsCount++
				}
			} else {
				errors = append(errors, PhotoError{
					ID:      photo.ID,
					URL:     photo.ImageURL,
					Error:   fmt.Sprintf("OCR error: %v", err),
					WebLink: webLink,
				})
			}
			continue
		}

		log.Printf("Photo %s: %s", photo.ID, text)

		// Update database if not in dry run mode
		if !*dryRun {
			updateQuery := "UPDATE photos SET title = ? WHERE id = ?"
			_, err := db.Exec(updateQuery, text, photo.ID)
			if err != nil {
				errors = append(errors, PhotoError{
					ID:      photo.ID,
					URL:     photo.ImageURL,
					Error:   fmt.Sprintf("Error updating database: %v", err),
					WebLink: webLink,
				})
				continue
			}
			updatedCount++
			log.Printf("Updated photo %s with new title: %s", photo.ID, text)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	fmt.Printf("Summary: Found %d photos, processed %d photos, updated %d photos, created %d review tasks\n",
		photoCount, processedCount, updatedCount, thingsCount)

	if len(errors) > 0 {
		fmt.Printf("\nErrors encountered (%d):\n", len(errors))
		for _, err := range errors {
			fmt.Printf("\nPhoto ID: %s\n", err.ID)
			fmt.Printf("\tImage URL: %s\n", err.URL)
			fmt.Printf("\tWeb UI: %s\n", err.WebLink)
			fmt.Printf("\tError: %s\n", err.Error)
		}
	}
}
