package elasticsearch

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v8"
	"go.uber.org/zap"

	"seattle_info_backend/internal/config"
)

// ESClientWrapper wraps the elasticsearch.Client.
// This can help Wire disambiguate types, especially from external modules.
type ESClientWrapper struct {
	*elasticsearch.Client
}

// ZapLogger is an adapter from zap.Logger to elastictransport.Logger.
type ZapLogger struct {
	logger *zap.Logger
}

// LogRoundTrip prints the request-response metrics.
// This is just an example; you might want to customize it further.
func (l *ZapLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	var (
		statusCode int
		reason     string
	)
	if res != nil {
		statusCode = res.StatusCode
	}
	if err != nil {
		reason = err.Error()
	}

	l.logger.Debug("Elasticsearch RoundTrip",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", dur),
		zap.Error(err),
		zap.String("reason", reason),
	)
	return nil
}

// RequestBodyEnabled makes the client pass a copy of request body to the logger.
func (l *ZapLogger) RequestBodyEnabled() bool { return true }

// ResponseBodyEnabled makes the client pass a copy of response body to the logger.
func (l *ZapLogger) ResponseBodyEnabled() bool { return true }

// NewClient creates and returns a new Elasticsearch client wrapper.
func NewClient(cfg *config.Config, logger *zap.Logger) (*ESClientWrapper, error) {
	if cfg.ElasticsearchURL == "" {
		// If ES is truly optional for the app, this could return (nil, nil)
		// but then services using it must handle a nil client.
		// For clearer dependency injection with Wire, especially if services
		// expect a client, it's better to return an error if config is missing.
		logger.Error("ElasticsearchURL is not configured. Elasticsearch client cannot be initialized.")
		return nil, fmt.Errorf("ElasticsearchURL is not configured in application config")
	}

	retryBackoff := func(i int) time.Duration {
		return time.Duration(i) * 100 * time.Millisecond
	}

	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.ElasticsearchURL},
		Logger:    &ZapLogger{logger: logger.Named("elasticsearch_client")}, // Attach the zap logger
		// Retry on 429 TooManyRequests statuses
		// Retry on 502 BadGateway, 503 ServiceUnavailable, 504 GatewayTimeout
		RetryOnStatus: []int{502, 503, 504, 429},
		// Configure the backoff function
		RetryBackoff: retryBackoff,
		// Retry up to 5 attempts
		MaxRetries: 5,
	}

	esClient, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		logger.Error("Error creating Elasticsearch client", zap.Error(err))
		return nil, fmt.Errorf("elasticsearch.NewClient: %w", err)
	}

	// Optional: Ping the Elasticsearch server to ensure connectivity
	res, err := esClient.Info()
	if err != nil {
		logger.Error("Error pinging Elasticsearch", zap.Error(err))
		return nil, fmt.Errorf("esClient.Info: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := decodeJSONResponse(res, &e); err != nil {
			logger.Error("Error decoding Elasticsearch error response", zap.Error(err), zap.String("status", res.Status()))
			return nil, fmt.Errorf("error decoding Elasticsearch error response: %s", res.Status())
		}
		logger.Error("Elasticsearch client initialization error", zap.String("status", res.Status()), zap.Any("error_details", e))
		return nil, fmt.Errorf("elasticsearch client initialization error: %s", res.Status())
	}

	logger.Info("Elasticsearch client initialized and connected successfully", zap.String("url", cfg.ElasticsearchURL), zap.String("es_version", elasticsearch.Version))
	return &ESClientWrapper{Client: esClient}, nil
}

// decodeJSONResponse is a helper to decode JSON responses, especially for errors.
// go-elasticsearch examples often include a similar helper.
func decodeJSONResponse(res *http.Response, target interface{}) error {
	if res.Body == nil {
		return fmt.Errorf("response body is nil")
	}
	// It's good practice to ensure the body is closed by the caller if not here.
	// However, since this is a helper, and res.Body.Close() is deferred in NewClient,
	// we avoid closing it here multiple times.
	// If this helper were used elsewhere, consider how body closing is handled.

	// Read the body
	bodyBytes, err :=ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	// Replace res.Body with a new reader so it can be read again if needed, though typically not for error decoding.
	res.Body = NopCloser(NewReader(bodyBytes))

	// Attempt to decode
	if err := NewDecoder(NewReader(bodyBytes)).Decode(target); err != nil {
		// If decoding fails, it might not be JSON or might be empty.
		// Log the raw body for debugging.
		return fmt.Errorf("failed to decode JSON response (body: %s): %w", string(bodyBytes), err)
	}
	return nil
}

// Helper functions to replace ioutil (deprecated) and allow re-reading response body
// In a real application, these might come from a utility package.

// ReadAll reads all data from r until EOF or error.
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// NopCloser returns a ReadCloser with a no-op Close method wrapping
// the provided Reader r.
func NopCloser(r io.Reader) io.ReadCloser {
	return io.NopCloser(r)
}

// NewReader returns a new strings.Reader reading from s.
// It is similar to bytes.NewBufferString but more direct for string input.
func NewReader(s []byte) *strings.Reader {
	return strings.NewReader(string(s))
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *json.Decoder {
	return json.NewDecoder(r)
}

// Required imports for the helpers
import (
	"encoding/json"
	"io"
)
