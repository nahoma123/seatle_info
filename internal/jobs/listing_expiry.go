// File: internal/jobs/listing_expiry.go
package jobs

import (
	"context"
	"fmt"
	"time"

	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/listing" // For listing.Service

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// ListingExpiryJob holds dependencies for the listing expiry job.
type ListingExpiryJob struct {
	listingService listing.Service
	logger         *zap.Logger
	cfg            *config.Config
	cronScheduler  *cron.Cron
}

// NewListingExpiryJob creates a new ListingExpiryJob.
func NewListingExpiryJob(
	listingService listing.Service,
	logger *zap.Logger,
	cfg *config.Config,
) *ListingExpiryJob {
	// cron.New(cron.WithSeconds()) // if you need second-level precision
	// cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))) // Skip if previous run is still active
	scheduler := cron.New(cron.WithLogger(NewCronLogger(logger.Named("cron"))))

	return &ListingExpiryJob{
		listingService: listingService,
		logger:         logger.Named("ListingExpiryJob"), // Named logger for context
		cfg:            cfg,
		cronScheduler:  scheduler,
	}
}

// SetupAndStart schedules and starts the cron job.
func (j *ListingExpiryJob) SetupAndStart() error {
	jobSpec := j.cfg.ListingExpiryJobSchedule // e.g., "@daily", "0 1 * * *" (1 AM daily)
	if jobSpec == "" {
		j.logger.Warn("Listing expiry job schedule not defined (LISTING_EXPIRY_JOB_SCHEDULE). Job will not run.")
		return nil // Not a fatal error, just won't run
	}

	jobID, err := j.cronScheduler.AddFunc(jobSpec, j.runJob)
	if err != nil {
		j.logger.Error("Failed to schedule listing expiry job", zap.String("spec", jobSpec), zap.Error(err))
		return err
	}

	j.logger.Info("Listing expiry job scheduled", zap.String("spec", jobSpec), zap.Any("jobID", jobID))
	j.cronScheduler.Start() // Start the scheduler in the background
	return nil
}

// runJob is the actual work performed by the cron job.
func (j *ListingExpiryJob) runJob() {
	j.logger.Info("Starting listing expiry job run...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // Job timeout
	defer cancel()

	expiredCount, err := j.listingService.ExpireListings(ctx)
	if err != nil {
		j.logger.Error("Listing expiry job run failed", zap.Error(err))
	} else {
		j.logger.Info("Listing expiry job run completed", zap.Int("listings_expired", expiredCount))
	}
}

// Stop gracefully stops the cron scheduler.
func (j *ListingExpiryJob) Stop() {
	if j.cronScheduler != nil {
		j.logger.Info("Stopping listing expiry job scheduler...")
		stopCtx := j.cronScheduler.Stop() // Returns a context that is done when the scheduler has stopped
		select {
		case <-stopCtx.Done():
			j.logger.Info("Listing expiry job scheduler stopped gracefully.")
		case <-time.After(10 * time.Second): // Timeout for stopping
			j.logger.Warn("Listing expiry job scheduler stop timed out.")
		}
	}
}

// --- Cron Logger Adapter ---

// cronLogger adapts zap.Logger to cron.Logger interface.
type cronLogger struct {
	zl *zap.Logger
}

// NewCronLogger creates a new cronLogger.
func NewCronLogger(zl *zap.Logger) cron.Logger {
	return &cronLogger{zl: zl}
}

// Info logs routine messages from cron.
func (cl *cronLogger) Info(msg string, keysAndValues ...interface{}) {
	fields := cl.parseKeysAndValues(keysAndValues...)
	cl.zl.Info(msg, fields...)
}

// Error logs error messages from cron.
func (cl *cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	fields := cl.parseKeysAndValues(keysAndValues...)
	fields = append(fields, zap.Error(err))
	cl.zl.Error(msg, fields...)
}

func (cl *cronLogger) parseKeysAndValues(keysAndValues ...interface{}) []zap.Field {
	var fields []zap.Field
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			fields = append(fields, zap.Any(fmt.Sprintf("%v", keysAndValues[i]), keysAndValues[i+1]))
		} else {
			fields = append(fields, zap.Any(fmt.Sprintf("%v", keysAndValues[i]), "MISSING_VALUE"))
		}
	}
	return fields
}
