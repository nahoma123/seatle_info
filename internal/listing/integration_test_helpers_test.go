package listing_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"seattle_info_backend/internal/listing" // Assuming your listing models are here
)

// CreateMultipartRequest constructs an HTTP request with multipart/form-data.
// params is a map of form field keys to string values.
// fileParams is a map of form field keys to file paths to be uploaded.
func CreateMultipartRequest(method, url string, params map[string]string, fileParams map[string][]string) (*http.Request, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// Add string parameters
	for key, val := range params {
		if strings.HasSuffix(key, "_json") { // Handle JSON string fields
			// It's important that the backend expects these as strings and unmarshals them.
			// The CreateListingRequest and UpdateListingRequest in model.go might need adjustment
			// if they expect direct struct binding for these from multipart forms.
			// Typically, for multipart, complex objects are sent as JSON strings.
			_ = writer.WriteField(key, val)
		} else {
			_ = writer.WriteField(key, val)
		}
	}

	// Add file parameters
	for key, paths := range fileParams {
		for _, path := range paths {
			file, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			part, err := writer.CreateFormFile(key, filepath.Base(path))
			if err != nil {
				return nil, fmt.Errorf("failed to create form file for %s: %w", path, err)
			}
			_, err = io.Copy(part, file)
			if err != nil {
				return nil, fmt.Errorf("failed to copy file content for %s: %w", path, err)
			}
		}
	}

	err := writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, nil
}


// createTempImageFile creates a dummy image file for testing uploads.
// Returns the path to the created file. Caller is responsible for deleting it.
func createTempImageFile(t interface{ TempDir() string; Errorf(string, ...interface{}) }, filename string, content []byte) string {
	tempDir := t.TempDir() // Go 1.15+ feature for creating temp dir for test
	// For older versions, use ioutil.TempDir or os.MkdirTemp

	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Errorf("Failed to create temp image file %s: %v", filePath, err)
		return ""
	}
	return filePath
}

// Helper to convert listing detail structs to JSON strings for multipart forms
func babysittingDetailsToJSON(t interface{ Errorf(string, ...interface{}) }, details *listing.CreateListingBabysittingDetailsRequest) string {
	if details == nil {
		return ""
	}
	b, err := json.Marshal(details)
	if err != nil {
		t.Errorf("Failed to marshal babysitting details: %v", err)
		return ""
	}
	return string(b)
}

func housingDetailsToJSON(t interface{ Errorf(string, ...interface{}) }, details *listing.CreateListingHousingDetailsRequest) string {
	if details == nil {
		return ""
	}
	b, err := json.Marshal(details)
	if err != nil {
		t.Errorf("Failed to marshal housing details: %v", err)
		return ""
	}
	return string(b)
}

func eventDetailsToJSON(t interface{ Errorf(string, ...interface{}) }, details *listing.CreateListingEventDetailsRequest) string {
	if details == nil {
		return ""
	}
	b, err := json.Marshal(details)
	if err != nil {
		t.Errorf("Failed to marshal event details: %v", err)
		return ""
	}
	return string(b)
}
