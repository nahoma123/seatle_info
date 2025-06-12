// File: cmd/server/main.go
package main

import (
	"context"
	"log" // Standard log for critical startup/shutdown messages before/after zap is active
	"os"
	"os/signal"
	"strings"
	"syscall"
	"flag" // Added for CLI flags
	"fmt"  // Added for printing

	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/listing" // Added for repository
	"seattle_info_backend/internal/listing/esutil" // Added for ListingToElasticsearchDoc
	"seattle_info_backend/internal/platform/database" // Added for direct DB connection
	"seattle_info_backend/internal/platform/logger"    // Added for direct logger
	platformElasticsearch "seattle_info_backend/internal/platform/elasticsearch"
	"go.uber.org/zap"
	"gorm.io/gorm" // Added for gorm.DB
	"github.com/elastic/go-elasticsearch/v8/esapi" // Added for Bulk API
)

func main() {
	// Define CLI flags
	syncListingsCmd := flag.NewFlagSet("sync-listings", flag.ExitOnError)
	batchSize := syncListingsCmd.Int("batch-size", 100, "Batch size for syncing listings")
	esRefresh := syncListingsCmd.String("es-refresh", "false", "Elasticsearch refresh policy (true, false, wait_for)")


	if len(os.Args) > 1 && os.Args[1] == "sync-listings" {
		syncListingsCmd.Parse(os.Args[2:]) // Parse flags for sync-listings command

		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("FATAL: Failed to load configuration for sync: %v", err)
		}
		appLogger, err := logger.New(cfg) // Initialize logger directly
		if err != nil {
			log.Fatalf("FATAL: Failed to initialize logger for sync: %v", err)
		}
		db, err := database.NewGORM(cfg) // Initialize DB directly
		if err != nil {
			appLogger.Fatal("FATAL: Failed to initialize database for sync", zap.Error(err))
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		esClient, err := platformElasticsearch.NewClient(cfg, appLogger)
		if err != nil {
			appLogger.Fatal("FATAL: Failed to initialize Elasticsearch client for sync", zap.Error(err))
		}
		if esClient == nil {
			appLogger.Fatal("FATAL: Elasticsearch client is nil though no error reported, ensure ELASTICSEARCH_URL is set.")
		}

		// Ensure index exists before syncing
		if err := platformElasticsearch.CreateListingsIndexIfNotExists(esClient, appLogger); err != nil {
			appLogger.Fatal("FATAL: Failed to create/verify Elasticsearch index before sync", zap.Error(err))
		}

		listingRepo := listing.NewGORMRepository(db) // Create repository

		err = runListingSync(cfg, listingRepo, esClient, appLogger, *batchSize, *esRefresh)
		if err != nil {
			appLogger.Fatal("FATAL: Listing synchronization failed", zap.Error(err))
		}
		appLogger.Info("Listing synchronization completed successfully.")
		return // Exit after sync command
	}

	// Default: Start server
	startServer()
}

func startServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}

	server, cleanup, err := initializeServer(cfg)
	if err != nil {
		log.Fatalf("FATAL: Failed to initialize server: %v", err)
	}
	defer cleanup()

	if server.ESClient != nil {
		if server.AppLogger == nil {
			log.Println("WARN: AppLogger is nil on server object, cannot create ES index. This is unexpected.")
		} else {
			if err := platformElasticsearch.CreateListingsIndexIfNotExists(server.ESClient, server.AppLogger); err != nil {
				server.AppLogger.Error("Failed to create Elasticsearch listings index. Depending on app logic, this might be fatal.", zap.Error(err))
			}
		}
	} else {
		if server.AppLogger != nil {
			server.AppLogger.Info("Elasticsearch client (server.ESClient) not initialized, skipping index creation.")
		} else {
			log.Println("INFO: Elasticsearch client (server.ESClient) not initialized, skipping index creation.")
		}
	}

	go func() {
		if err := server.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("FATAL: Server failed to start or crashed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("INFO: Received signal '%s'. Shutting down server...", sig)

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ServerTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("ERROR: Server forced to shutdown due to error: %v", err)
	} else {
		log.Println("INFO: Server shutdown complete.")
	}
	log.Println("INFO: Application exiting.")
}


// runListingSync performs the batch synchronization of listings to Elasticsearch.
func runListingSync(
	cfg *config.Config,
	listingRepo listing.Repository, // Use the repository interface
	esClient *platformElasticsearch.ESClientWrapper,
	logger *zap.Logger,
	batchSize int,
	esRefresh string,
) error {
	logger.Info("Starting listing synchronization to Elasticsearch...",
		zap.Int("batchSize", batchSize),
		zap.String("esRefreshPolicy", esRefresh),
	)

	offset := 0
	totalSynced := 0
	totalFailed := 0
	batchNumber := 1

	for {
		logger.Info("Fetching batch of listings...", zap.Int("batchNumber", batchNumber), zap.Int("offset", offset), zap.Int("limit", batchSize))
		listings, err := listingRepo.FindAllForSync(context.Background(), offset, batchSize)
		if err != nil {
			logger.Error("Failed to fetch batch of listings", zap.Error(err), zap.Int("batchNumber", batchNumber))
			return fmt.Errorf("failed to fetch batch %d: %w", batchNumber, err)
		}

		if len(listings) == 0 {
			logger.Info("No more listings to sync.")
			break
		}
		logger.Info("Fetched listings for batch", zap.Int("count", len(listings)), zap.Int("batchNumber", batchNumber))


		var bulkRequestBody strings.Builder
		currentBatchIDs := make([]string, 0, len(listings))

		for i := range listings {
			l := &listings[i] // Use a pointer to the listing for ListingToElasticsearchDoc
			currentBatchIDs = append(currentBatchIDs, l.ID.String())
			docJSON, errDoc := esutil.ListingToElasticsearchDoc(l)
			if errDoc != nil {
				logger.Error("Failed to convert listing to Elasticsearch document",
					zap.String("listingID", l.ID.String()),
					zap.Error(errDoc),
				)
				totalFailed++
				continue // Skip this document
			}

			action := fmt.Sprintf(`{ "index" : { "_index" : "%s", "_id" : "%s" } }%s`, platformElasticsearch.ListingsIndexName, l.ID.String(), "\n")
			bulkRequestBody.WriteString(action)
			bulkRequestBody.WriteString(docJSON)
			bulkRequestBody.WriteString("\n")
		}

		if bulkRequestBody.Len() == 0 {
			logger.Info("No documents to index in current batch, possibly due to conversion errors.", zap.Int("batchNumber", batchNumber))
			offset += len(listings) // Still advance offset
			batchNumber++
			continue
		}

		logger.Info("Sending bulk request to Elasticsearch for batch", zap.Int("batchNumber", batchNumber), zap.Int("documentCount", len(currentBatchIDs)))

		req := esapi.BulkRequest{
			Body:    strings.NewReader(bulkRequestBody.String()),
			Refresh: esRefresh,
		}

		res, errBulk := req.Do(context.Background(), esClient.Client)
		if errBulk != nil {
			logger.Error("Failed to send bulk request to Elasticsearch", zap.Error(errBulk), zap.Int("batchNumber", batchNumber))
			totalFailed += len(currentBatchIDs) // Assume all in batch failed if request itself failed
			// Optionally, could implement retry logic here
			offset += len(listings)
			batchNumber++
			continue // Or return error to stop sync
		}
		defer res.Body.Close()

		batchSynced := 0
		batchFailed := 0

		if res.IsError() {
			logger.Error("Elasticsearch bulk request returned an error", zap.String("status", res.Status()), zap.Int("batchNumber", batchNumber))
			// Try to parse the response to see individual item errors
			var raw map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
				logger.Error("Failed to parse Elasticsearch bulk error response body", zap.Error(err))
				totalFailed += len(currentBatchIDs)
			} else {
				if errors, ok := raw["errors"].(bool); ok && errors {
					items, _ := raw["items"].([]interface{})
					for i, item := range items {
						itemMap, _ := item.(map[string]interface{})
						indexMap, _ := itemMap["index"].(map[string]interface{}) // Or "create", "update", "delete"

						listingID := "unknown"
						if i < len(currentBatchIDs) {
							listingID = currentBatchIDs[i]
						}

						if errorVal, ok := indexMap["error"]; ok {
							logger.Error("Failed to index document in bulk batch",
								zap.String("listingID", listingID),
								zap.Any("error", errorVal),
								zap.Int("batchItemIndex", i),
							)
							batchFailed++
						} else {
							batchSynced++
						}
					}
				} else { // Bulk request itself error, but not specific item errors found in expected format
					  logger.Warn("Elasticsearch bulk request had IsError() true but no 'errors:true' field in response or items array missing", zap.Int("batchNumber", batchNumber))
            totalFailed += len(currentBatchIDs)
				}
			}
		} else { // No error from res.IsError()
			// Still need to check item-level errors as bulk can succeed overall but have individual failures
			var bulkResponse struct {
				Errors bool `json:"errors"`
				Items  []struct {
					Index struct {
						ID     string                 `json:"_id"`
						Status int                    `json:"status"`
						Error  map[string]interface{} `json:"error,omitempty"`
					} `json:"index"`
				} `json:"items"`
			}
			if err := json.NewDecoder(res.Body).Decode(&bulkResponse); err != nil {
				logger.Error("Failed to parse Elasticsearch bulk success response body", zap.Error(err), zap.Int("batchNumber", batchNumber))
				totalFailed += len(currentBatchIDs) // Assume all failed if can't parse response
			} else {
				for _, item := range bulkResponse.Items {
					if item.Index.Error != nil {
						logger.Error("Failed to index document in bulk batch (item-level)",
							zap.String("listingID", item.Index.ID),
							zap.Any("error", item.Index.Error),
							zap.Int("status", item.Index.Status),
						)
						batchFailed++
					} else {
						batchSynced++
					}
				}
			}
		}

		totalSynced += batchSynced
		totalFailed += batchFailed
		logger.Info("Batch processed.",
			zap.Int("batchNumber", batchNumber),
			zap.Int("syncedInBatch", batchSynced),
			zap.Int("failedInBatch", batchFailed),
		)

		offset += len(listings)
		batchNumber++
	}

	logger.Info("Listing synchronization process finished.",
		zap.Int("totalListingsSyncedSuccessfully", totalSynced),
		zap.Int("totalListingsFailed", totalFailed),
	)
	if totalFailed > 0 {
		return fmt.Errorf("%d listings failed to sync", totalFailed)
	}
	return nil
}
