package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"
)

const ListingsIndexName = "listings"

// defineListingsMapping returns the JSON string for the listings index mapping.
func defineListingsMapping() (string, error) {
	mapping := map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"title":             map[string]interface{}{"type": "text"},
				"description":       map[string]interface{}{"type": "text"},
				"location":          map[string]interface{}{"type": "geo_point"}, // For lat, lon
				"category_id":       map[string]interface{}{"type": "keyword"},
				"sub_category_id":   map[string]interface{}{"type": "keyword"},
				"user_id":           map[string]interface{}{"type": "keyword"},
				"status":            map[string]interface{}{"type": "keyword"},
				"expires_at":        map[string]interface{}{"type": "date"},
				"created_at":        map[string]interface{}{"type": "date"},
				"updated_at":        map[string]interface{}{"type": "date"},    // Added for completeness
				"is_admin_approved": map[string]interface{}{"type": "boolean"},
				// Additional filterable/sortable fields from ListingResponse
				"contact_name":  map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": {"type": "keyword", "ignore_above": 256}}},
				"city":          map[string]interface{}{"type": "keyword"},
				"state":         map[string]interface{}{"type": "keyword"},
				"zip_code":      map[string]interface{}{"type": "keyword"},
				"address_line1": map[string]interface{}{"type": "text"}, // Good for partial search if needed
				// Babysitting specific
				"languages_spoken": map[string]interface{}{"type": "keyword"}, // Assuming a list of keywords
				// Housing specific
				"property_type": map[string]interface{}{"type": "keyword"},
				"rent_details":  map[string]interface{}{"type": "text"},    // Can be keyword if exact match is always used
				"sale_price":    map[string]interface{}{"type": "double"}, // Use appropriate numeric type
				// Event specific
				"event_date":     map[string]interface{}{"type": "date"},
				"event_time":     map[string]interface{}{"type": "keyword"}, // Or date if parsing and storing as time
				"organizer_name": map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": {"type": "keyword", "ignore_above": 256}}},
				"venue_name":     map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": {"type": "keyword", "ignore_above": 256}}},
			},
		},
	}
	mappingBytes, err := json.Marshal(mapping)
	if err != nil {
		return "", fmt.Errorf("error marshalling listings mapping to JSON: %w", err)
	}
	return string(mappingBytes), nil
}

// CreateListingsIndexIfNotExists creates the listings index with the defined mapping
// if it does not already exist.
func CreateListingsIndexIfNotExists(client *ESClientWrapper, logger *zap.Logger) error {
	ctx := context.Background()
	log := logger.Named("elasticsearch_index_setup")

	// 1. Check if the index exists
	req := esapi.IndicesExistsRequest{
		Index: []string{ListingsIndexName},
	}
	res, err := req.Do(ctx, client.Client) // Use client.Client to access the underlying *elasticsearch.Client
	if err != nil {
		log.Error("Error checking if listings index exists", zap.Error(err))
		return fmt.Errorf("error checking if listings index exists: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		log.Info("Listings index already exists", zap.String("index_name", ListingsIndexName))
		return nil
	}
	if res.StatusCode != http.StatusNotFound {
		log.Error("Error checking if listings index exists, unexpected status",
			zap.String("status", res.Status()),
			zap.String("index_name", ListingsIndexName),
		)
		return fmt.Errorf("error checking if listings index exists: status %s", res.Status())
	}

	// 2. Define the mapping
	mappingJSON, err := defineListingsMapping()
	if err != nil {
		log.Error("Failed to define listings mapping", zap.Error(err))
		return err
	}
	log.Debug("Listings index mapping defined", zap.String("mapping", mappingJSON))

	// 3. Create the index
	createReq := esapi.IndicesCreateRequest{
		Index: ListingsIndexName,
		Body:  strings.NewReader(mappingJSON),
	}
	createRes, err := createReq.Do(ctx, client.Client)
	if err != nil {
		log.Error("Error creating listings index", zap.Error(err), zap.String("index_name", ListingsIndexName))
		return fmt.Errorf("error creating listings index %s: %w", ListingsIndexName, err)
	}
	defer createRes.Body.Close()

	if createRes.IsError() {
		var errorBody map[string]interface{}
		if err := json.NewDecoder(createRes.Body).Decode(&errorBody); err != nil {
			log.Error("Failed to parse listings index creation error response body", zap.Error(err), zap.String("status", createRes.Status()))
		} else {
			log.Error("Failed to create listings index",
				zap.String("status", createRes.Status()),
				zap.Any("error_details", errorBody),
				zap.String("index_name", ListingsIndexName),
			)
		}
		return fmt.Errorf("failed to create listings index %s: status %s", ListingsIndexName, createRes.Status())
	}

	log.Info("Listings index created successfully", zap.String("index_name", ListingsIndexName))
	return nil
}

// Helper to read response body as string, useful for debugging
func responseBodyToString(res *esapi.Response) string {
	if res == nil || res.Body == nil {
		return ""
	}
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		return fmt.Sprintf("failed to read response body: %v", err)
	}
	return buf.String()
}
