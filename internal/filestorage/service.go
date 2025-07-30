package filestorage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FileStorageService provides operations for storing and deleting files.
type FileStorageService struct {
	storagePath string // Base path for storing files, e.g., "./images"
	logger      *zap.Logger
}

// NewFileStorageService creates a new FileStorageService.
func NewFileStorageService(storagePath string, logger *zap.Logger) (*FileStorageService, error) {
	if storagePath == "" {
		return nil, fmt.Errorf("storage path cannot be empty")
	}
	// Ensure the base storage path exists
	if err := os.MkdirAll(storagePath, os.ModePerm); err != nil {
		logger.Error("Failed to create storage path directory", zap.String("path", storagePath), zap.Error(err))
		return nil, fmt.Errorf("failed to create storage path %s: %w", storagePath, err)
	}
	logger.Info("FileStorageService initialized", zap.String("storagePath", storagePath))
	return &FileStorageService{storagePath: storagePath, logger: logger}, nil
}

// SaveUploadedFile saves a multipart file to a specified sub-directory within the storage path.
// It generates a unique filename using UUID.
// subDir is relative to the base storagePath, e.g., "listings", "avatars".
// Returns the relative path of the saved file (e.g., "listings/uuid.jpg") or an error.
func (s *FileStorageService) SaveUploadedFile(fileHeader *multipart.FileHeader, subDir string) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("fileHeader cannot be nil")
	}

	src, err := fileHeader.Open()
	if err != nil {
		s.logger.Error("Failed to open uploaded file", zap.Error(err))
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Generate a unique filename
	originalFilename := filepath.Base(fileHeader.Filename)
	extension := filepath.Ext(originalFilename)
	if extension == "" {
		// Attempt to infer extension from content type if possible, or default
		// For simplicity, let's require files to have extensions or set a default
		// This part can be enhanced with MIME type detection for more robustness
		contentType := fileHeader.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(contentType, "image/jpeg"):
			extension = ".jpg"
		case strings.HasPrefix(contentType, "image/png"):
			extension = ".png"
		case strings.HasPrefix(contentType, "image/gif"):
			extension = ".gif"
		default:
			return "", fmt.Errorf("unsupported file type or missing extension: %s", contentType)
		}

	}
	uniqueFilename := uuid.New().String() + extension

	// Construct the full destination path
	// Ensure subDir is clean and does not try to escape storagePath (though less critical here as it's not user input)
	cleanSubDir := filepath.Clean(subDir)
	if strings.HasPrefix(cleanSubDir, "..") { // Basic check
		s.logger.Error("Invalid subDir, attempts to navigate up", zap.String("subDir", subDir))
		return "", fmt.Errorf("invalid subDir path")
	}

	destinationDir := filepath.Join(s.storagePath, cleanSubDir)
	if err := os.MkdirAll(destinationDir, os.ModePerm); err != nil {
		s.logger.Error("Failed to create sub-directory for file storage", zap.String("path", destinationDir), zap.Error(err))
		return "", fmt.Errorf("failed to create directory %s: %w", destinationDir, err)
	}

	destinationPath := filepath.Join(destinationDir, uniqueFilename)

	dst, err := os.Create(destinationPath)
	if err != nil {
		s.logger.Error("Failed to create destination file", zap.String("path", destinationPath), zap.Error(err))
		return "", fmt.Errorf("failed to create file %s: %w", destinationPath, err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		s.logger.Error("Failed to copy uploaded file to destination", zap.String("path", destinationPath), zap.Error(err))
		// Attempt to remove partially written file
		os.Remove(destinationPath)
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	s.logger.Info("File saved successfully", zap.String("path", destinationPath))
	// Return path relative to the storagePath's subDir, e.g., "listings/uuid.jpg"
	return filepath.ToSlash(filepath.Join(cleanSubDir, uniqueFilename)), nil
}

// DeleteFile deletes a file given its path relative to the storagePath.
// relativePath is e.g., "listings/uuid.jpg".
func (s *FileStorageService) DeleteFile(relativePath string) error {
	if relativePath == "" {
		return fmt.Errorf("relative path cannot be empty")
	}

	// Clean the relative path to prevent path traversal vulnerabilities
	// For example, if relativePath was "../../../etc/passwd"
	// filepath.Join(s.storagePath, relativePath) might resolve to an unintended path.
	// A more robust approach is to ensure relativePath does not contain ".."
	cleanRelativePath := filepath.Clean(relativePath)
	if strings.Contains(cleanRelativePath, "..") {
		s.logger.Warn("Attempt to delete file with path traversal", zap.String("relativePath", relativePath))
		return fmt.Errorf("invalid file path for deletion")
	}

	fullPath := filepath.Join(s.storagePath, cleanRelativePath)

	// Check if file exists before attempting to delete
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.logger.Warn("Attempt to delete non-existent file", zap.String("path", fullPath))
		return nil // Or return an error if strictness is required: fmt.Errorf("file not found: %s", fullPath)
	}

	if err := os.Remove(fullPath); err != nil {
		s.logger.Error("Failed to delete file", zap.String("path", fullPath), zap.Error(err))
		return fmt.Errorf("failed to delete file %s: %w", fullPath, err)
	}

	s.logger.Info("File deleted successfully", zap.String("path", fullPath))
	return nil
}