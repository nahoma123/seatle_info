package filestorage

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// "seattle_info_backend/internal/platform/logger" // No longer needed due to zap.NewNop()
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap" // Added import for zap.NewNop()
)

const testStoragePath = "./test_images_temp"

func setupFileStorageService(t *testing.T) (*FileStorageService, func()) {
	err := os.MkdirAll(testStoragePath, os.ModePerm)
	require.NoError(t, err, "Failed to create test storage path")

	zapLogger := zap.NewNop() // Use a Nop logger for simple unit tests
	fsService, err := NewFileStorageService(testStoragePath, zapLogger)
	require.NoError(t, err, "Failed to create FileStorageService")
	require.NotNil(t, fsService)

	cleanup := func() {
		err := os.RemoveAll(testStoragePath)
		if err != nil {
			t.Logf("Warning: Failed to remove test storage path %s: %v", testStoragePath, err)
		}
	}
	return fsService, cleanup
}

// Helper to create a valid multipart.FileHeader that can be opened
// This function replaces the old createMockFileHeader
func newTestFileHeader(t *testing.T, fieldname, filename, content, contentType string) *multipart.FileHeader {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename))
	if contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}

	part, err := writer.CreatePart(partHeader)
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader(content))
	require.NoError(t, err)
	writer.Close() // Important: Close writer to finalize the body

	// Now, use multipart.NewReader to parse this body and extract the FileHeader
	// This simulates how Gin would process the incoming multipart request
	reader := multipart.NewReader(body, writer.Boundary())
	form, err := reader.ReadForm(32 << 20) // Max memory
	require.NoError(t, err)

	files := form.File[fieldname]
	require.NotEmpty(t, files, "No files found for fieldname %s", fieldname)
	return files[0]
}


func TestFileStorageService_SaveUploadedFile_Success(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	mockContent := "This is a test image file."
	mockFilename := "test_image.jpg"
	mockContentType := "image/jpeg"

	fh := newTestFileHeader(t, "upload", mockFilename, mockContent, mockContentType)

	subDir := "listings_test"
	relativePath, err := fsService.SaveUploadedFile(fh, subDir)

	require.NoError(t, err)
	assert.NotEmpty(t, relativePath)
	assert.True(t, strings.HasPrefix(relativePath, subDir+"/"), "Relative path should start with subDir")
	assert.True(t, strings.HasSuffix(relativePath, ".jpg"), "Relative path should end with .jpg extension")

	// Verify file exists and content is correct
	fullPath := filepath.Join(testStoragePath, relativePath)
	_, err = os.Stat(fullPath)
	assert.NoError(t, err, "File should exist at the returned path")

	fileContent, err := os.ReadFile(fullPath)
	assert.NoError(t, err)
	assert.Equal(t, mockContent, string(fileContent))
}

func TestFileStorageService_SaveUploadedFile_UnsupportedType(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	fh := newTestFileHeader(t, "upload", "test_document.txt", "some text", "text/plain") // .txt is not configured as image

	_, err := fsService.SaveUploadedFile(fh, "documents_test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file type or missing extension", "Error message should indicate unsupported type")
}

func TestFileStorageService_SaveUploadedFile_NoExtensionFallback(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	// Test with a filename that has no extension, but a known content type
	fhPNG := newTestFileHeader(t, "upload", "imagepng", "png content", "image/png")
	relPathPNG, errPNG := fsService.SaveUploadedFile(fhPNG, "no_ext_test")
	require.NoError(t, errPNG)
	assert.True(t, strings.HasSuffix(relPathPNG, ".png"))
	fullPathPNG := filepath.Join(testStoragePath, relPathPNG)
	_, err := os.Stat(fullPathPNG)
	assert.NoError(t, err, "File should exist for PNG with inferred extension")


	fhJPG := newTestFileHeader(t, "upload", "imagejpeg", "jpeg content", "image/jpeg")
	relPathJPG, errJPG := fsService.SaveUploadedFile(fhJPG, "no_ext_test")
	require.NoError(t, errJPG)
	assert.True(t, strings.HasSuffix(relPathJPG, ".jpg"))
	fullPathJPG := filepath.Join(testStoragePath, relPathJPG)
	_, err = os.Stat(fullPathJPG)
	assert.NoError(t, err, "File should exist for JPG with inferred extension")

}


func TestFileStorageService_DeleteFile_Success(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	// First, save a file
	mockContent := "temporary content for deletion test"
	mockFilename := "file_to_delete.txt"
	// Manually create a file as SaveUploadedFile has content type checks now
	subDir := "delete_test"
	tempFilePath := filepath.Join(testStoragePath, subDir, mockFilename)
	require.NoError(t, os.MkdirAll(filepath.Join(testStoragePath, subDir), os.ModePerm))
	require.NoError(t, os.WriteFile(tempFilePath, []byte(mockContent), 0644))

	relativePath := filepath.ToSlash(filepath.Join(subDir, mockFilename))

	// Verify it exists
	_, err := os.Stat(tempFilePath)
	require.NoError(t, err, "File should exist before deletion attempt")

	err = fsService.DeleteFile(relativePath)
	require.NoError(t, err)

	// Verify file no longer exists
	_, err = os.Stat(tempFilePath)
	assert.True(t, os.IsNotExist(err), "File should not exist after deletion")
}

func TestFileStorageService_DeleteFile_NonExistent(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	err := fsService.DeleteFile("non_existent_subdir/non_existent_file.txt")
	assert.NoError(t, err, "Deleting a non-existent file should not error by default, or return a specific 'not found' error if strict")
	// Current implementation logs a warning and returns nil.
}

func TestFileStorageService_DeleteFile_PathTraversal(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	// Attempt to delete a file outside the storage path
	// This test depends on the robustness of filepath.Clean and the checks in DeleteFile
	// Create a dummy file at root to simulate something outside
	dummyFilePath := filepath.Join(testStoragePath, "../dummy_outside.txt")
	os.WriteFile(dummyFilePath, []byte("dummy"), 0644)
	defer os.Remove(dummyFilePath)


	err := fsService.DeleteFile("../../dummy_outside.txt")
	require.Error(t, err, "Should not be able to delete files outside storage path")
	assert.Contains(t, err.Error(), "invalid file path for deletion")

	// Ensure the dummy file was not deleted
	_, statErr := os.Stat(dummyFilePath)
	assert.NoError(t, statErr, "External dummy file should still exist.")
}

func TestFileStorageService_SaveUploadedFile_NilHeader(t *testing.T) {
	fsService, cleanup := setupFileStorageService(t)
	defer cleanup()

	_, err := fsService.SaveUploadedFile(nil, "test_dir")
	assert.Error(t, err)
	assert.EqualError(t, err, "fileHeader cannot be nil")
}
