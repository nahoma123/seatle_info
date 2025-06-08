// File: internal/listing/repository.go
package listing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"seattle_info_backend/internal/common"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository defines the interface for listing data operations.
type Repository interface {
	Create(ctx context.Context, listing *Listing) error
	FindByID(ctx context.Context, id uuid.UUID, preloadAssociations bool) (*Listing, error)
	Update(ctx context.Context, listing *Listing) error
	Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error // UserID for ownership check
	Search(ctx context.Context, query ListingSearchQuery) ([]Listing, *common.Pagination, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) error
	FindExpiredListings(ctx context.Context, now time.Time) ([]Listing, error)
	CountListingsByUserIDAndStatus(ctx context.Context, userID uuid.UUID, status ListingStatus) (int64, error)
	CountListingsByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	GetRecentListings(ctx context.Context, page, pageSize int, currentUserID *uuid.UUID) ([]Listing, *common.Pagination, error)
	GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]Listing, *common.Pagination, error)
}

// GORMRepository implements the listing Repository interface using GORM.
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a new GORM listing repository.
func NewGORMRepository(db *gorm.DB) Repository {
	return &GORMRepository{db: db}
}

// preloader applies common preloads for listings.
func (r *GORMRepository) preloader(query *gorm.DB) *gorm.DB {
	return query.Preload("User").
		Preload("Category").
		Preload("SubCategory").
		Preload("BabysittingDetails").
		Preload("HousingDetails").
		Preload("EventDetails")
}

// Create inserts a new listing and its details into the database within a transaction.
func (r *GORMRepository) Create(ctx context.Context, listing *Listing) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create the main listing record
		if err := tx.Create(listing).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
				return common.ErrConflict.WithDetails("A similar listing might already exist or a unique constraint was violated.")
			}
			return fmt.Errorf("failed to create listing: %w", err)
		}

		// Create details if they exist
		if listing.BabysittingDetails != nil {
			listing.BabysittingDetails.ListingID = listing.ID
			if err := tx.Create(listing.BabysittingDetails).Error; err != nil {
				return fmt.Errorf("failed to create babysitting details: %w", err)
			}
		}
		if listing.HousingDetails != nil {
			listing.HousingDetails.ListingID = listing.ID
			if err := tx.Create(listing.HousingDetails).Error; err != nil {
				return fmt.Errorf("failed to create housing details: %w", err)
			}
		}
		if listing.EventDetails != nil {
			listing.EventDetails.ListingID = listing.ID
			if err := tx.Create(listing.EventDetails).Error; err != nil {
				return fmt.Errorf("failed to create event details: %w", err)
			}
		}
		return nil
	})
}

// FindByID retrieves a listing by its ID.
func (r *GORMRepository) FindByID(ctx context.Context, id uuid.UUID, preloadAssociations bool) (*Listing, error) {
	var listing Listing
	query := r.db.WithContext(ctx)
	if preloadAssociations {
		query = r.preloader(query)
	}
	err := query.First(&listing, "listings.id = ?", id).Error // Specify listings.id to avoid ambiguity if joining
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("Listing not found.")
		}
		return nil, err
	}
	return &listing, nil
}

// Update modifies an existing listing and its details in the database within a transaction.
func (r *GORMRepository) Update(ctx context.Context, listing *Listing) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Save the main listing record. .Save updates all fields or inserts if not found by primary key.
		// Use .Model(&Listing{}).Where("id = ?", listing.ID).Updates(map_of_changes) for partial updates.
		// For simplicity with full struct, Save is used.
		if err := tx.Save(listing).Error; err != nil {
			return fmt.Errorf("failed to update listing: %w", err)
		}

		// Update or Create details
		// GORM's .Save on associations can be tricky, explicit .Updates or .Create might be safer
		// or delete existing and recreate. For simplicity, we assume .Save on the main listing
		// with preloaded or assigned associations might work if GORM is configured for it,
		// but it's often more robust to handle them explicitly.

		// Example: Upsert Babysitting Details
		if listing.BabysittingDetails != nil {
			listing.BabysittingDetails.ListingID = listing.ID
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "listing_id"}},
				DoUpdates: clause.AssignmentColumns(getUpdatableColumns(ListingDetailsBabysitting{})),
			}).Create(listing.BabysittingDetails).Error; err != nil {
				return fmt.Errorf("failed to upsert babysitting details: %w", err)
			}
		} else {
			// If details are nil, it might mean they should be deleted
			tx.Where("listing_id = ?", listing.ID).Delete(&ListingDetailsBabysitting{})
		}

		if listing.HousingDetails != nil {
			listing.HousingDetails.ListingID = listing.ID
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "listing_id"}},
				DoUpdates: clause.AssignmentColumns(getUpdatableColumns(ListingDetailsHousing{})),
			}).Create(listing.HousingDetails).Error; err != nil {
				return fmt.Errorf("failed to upsert housing details: %w", err)
			}
		} else {
			tx.Where("listing_id = ?", listing.ID).Delete(&ListingDetailsHousing{})
		}

		if listing.EventDetails != nil {
			listing.EventDetails.ListingID = listing.ID
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "listing_id"}},
				DoUpdates: clause.AssignmentColumns(getUpdatableColumns(ListingDetailsEvents{})),
			}).Create(listing.EventDetails).Error; err != nil {
				return fmt.Errorf("failed to upsert event details: %w", err)
			}
		} else {
			tx.Where("listing_id = ?", listing.ID).Delete(&ListingDetailsEvents{})
		}

		return nil
	})
}

// getUpdatableColumns inspects a struct and returns a list of its field names, excluding primary key.
// This is a helper for clause.AssignmentColumns.
func getUpdatableColumns(model interface{}) []string {
	var fieldNames []string
	// This is a simplified example. A more robust way would use reflection
	// to get GORM field names, excluding 'listing_id' or other primary/foreign keys.
	// For now, list them manually based on your models.
	switch model.(type) {
	case ListingDetailsBabysitting:
		fieldNames = []string{"languages_spoken"}
	case ListingDetailsHousing:
		fieldNames = []string{"property_type", "rent_details", "sale_price"}
	case ListingDetailsEvents:
		fieldNames = []string{"event_date", "event_time", "organizer_name", "venue_name"}
	}
	return fieldNames
}

// Delete removes a listing by ID, ensuring ownership.
func (r *GORMRepository) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	// First, check if the listing exists and belongs to the user
	var listing Listing
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&listing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.ErrNotFound.WithDetails("Listing not found or you do not have permission to delete it.")
		}
		return err
	}

	// Deleting the main listing will cascade delete its details due to DB constraints
	result := r.db.WithContext(ctx).Select(clause.Associations).Delete(&Listing{BaseModel: common.BaseModel{ID: id}})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// This case should ideally be caught by the First() call above.
		return common.ErrNotFound.WithDetails("Listing not found or already deleted.")
	}
	return nil
}

// Search retrieves listings based on query parameters, including location-based search.
func (r *GORMRepository) Search(ctx context.Context, queryParams ListingSearchQuery) ([]Listing, *common.Pagination, error) {
	var listings []Listing
	var totalItems int64

	dbQuery := r.db.WithContext(ctx).Model(&Listing{})
	dbQuery = r.preloader(dbQuery) // Apply preloads

	// --- Apply Filters ---
	if queryParams.SearchTerm != "" {
		searchTerm := "%" + strings.ToLower(queryParams.SearchTerm) + "%"
		dbQuery = dbQuery.Where("LOWER(listings.title) LIKE ? OR LOWER(listings.description) LIKE ?", searchTerm, searchTerm)
	}
	if queryParams.CategoryID != nil && *queryParams.CategoryID != "" {
		dbQuery = dbQuery.Where("listings.category_id = ?", *queryParams.CategoryID)
	}
	if queryParams.SubCategoryID != nil && *queryParams.SubCategoryID != "" {
		dbQuery = dbQuery.Where("listings.sub_category_id = ?", *queryParams.SubCategoryID)
	}
	if queryParams.UserID != nil && *queryParams.UserID != "" {
		dbQuery = dbQuery.Where("listings.user_id = ?", *queryParams.UserID)
	}
	if queryParams.Status != "" {
		dbQuery = dbQuery.Where("listings.status = ?", queryParams.Status)
	} else if !queryParams.IncludeExpired {
		// Default: only show active or pending, exclude expired unless explicitly requested
		dbQuery = dbQuery.Where("listings.status IN (?)", []ListingStatus{StatusActive, StatusPendingApproval})
		dbQuery = dbQuery.Where("listings.expires_at > ?", time.Now())
	}

	// Location-based filtering and sorting
	// Using ST_DWithin for distance filtering and ST_Distance for sorting by distance.
	// These require PostGIS functions.
	if queryParams.Latitude != nil && queryParams.Longitude != nil {
		userLocation := fmt.Sprintf("SRID=4326;POINT(%f %f)", *queryParams.Longitude, *queryParams.Latitude)

		if queryParams.MaxDistanceKM != nil && *queryParams.MaxDistanceKM > 0 {
			maxDistanceMeters := *queryParams.MaxDistanceKM * 1000
			// ST_DWithin checks if geometries are within a certain distance (in meters for geography).
			dbQuery = dbQuery.Where("ST_DWithin(listings.location, ST_GeographyFromText(?), ?)", userLocation, maxDistanceMeters)
		}

		// Add distance calculation to the select clause if sorting by distance or for display
		// The alias 'distance_km' can be used in sorting and will be scanned into the ListingResponse.
		// Note: GORM might not directly scan into a non-model field. This might require a custom struct for results or careful handling.
		// For simplicity, we might just sort and rely on frontend to know the user's location if distance display is needed.
		// Or, we can add a 'Distance' field to Listing model with `gorm:"-"` (not a DB column) and populate it.
		// Let's assume for now we just sort by it. For displaying, it would need a Scan.
		if queryParams.SortBy == "distance" {
			// ST_Distance returns distance in meters for geography type.
			dbQuery = dbQuery.Order(gorm.Expr("ST_Distance(listings.location, ST_GeographyFromText(?))", userLocation))
		}
	}

	// --- Count Total Items for Pagination (before applying limit/offset) ---
	if err := dbQuery.Count(&totalItems).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to count listings: %w", err)
	}

	// --- Apply Sorting (other than distance) ---
	if queryParams.SortBy != "" && queryParams.SortBy != "distance" { // Distance sorting handled above
		sortOrder := "ASC"
		if strings.ToLower(queryParams.SortOrder) == "desc" {
			sortOrder = "DESC"
		}
		// Sanitize SortBy to prevent SQL injection if it's user-provided and not from a fixed list
		// For now, assume SortBy is from a controlled set (e.g., "created_at", "expires_at", "title")
		// Valid SortBy fields should be actual column names.
		validSortableFields := map[string]string{
			"created_at": "listings.created_at",
			"expires_at": "listings.expires_at",
			"title":      "listings.title",
			// Add more as needed
		}
		if dbSortField, ok := validSortableFields[queryParams.SortBy]; ok {
			dbQuery = dbQuery.Order(fmt.Sprintf("%s %s", dbSortField, sortOrder))
		} else {
			// Default sort if SortBy is invalid or not "distance"
			dbQuery = dbQuery.Order("listings.created_at DESC")
		}
	} else if queryParams.SortBy != "distance" { // Default sort if no sort_by is specified
		dbQuery = dbQuery.Order("listings.created_at DESC")
	}
	// Secondary sort for proximity (BR2.1: if distance is primary, recency is secondary)
	// If sorting by distance, we can add a secondary sort by created_at DESC.
	if queryParams.SortBy == "distance" {
		dbQuery = dbQuery.Order("listings.created_at DESC") // This adds to existing order by distance
	}

	// --- Apply Pagination ---
	pagination := common.NewPagination(totalItems, queryParams.Page, queryParams.PageSize)
	dbQuery = dbQuery.Offset(pagination.CurrentPage - 1*pagination.PageSize).Limit(pagination.PageSize) // Correct offset calculation

	dbQuery = dbQuery.
		Omit("location").                                         // ① drop geometry
		Select("listings.*, ST_AsText(location) AS location_wkt") // ② add WKT

	for i := range listings {
		if listings[i].LocationWKT != "" {
			point, err := parseWKT(listings[i].LocationWKT)
			if err != nil {
				// Log or handle error properly
				fmt.Printf("Failed to parse WKT: %v\n", err)
				continue
			}
			listings[i].Location = point
		}
	}

	if err := dbQuery.Find(&listings).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to search listings: %w", err)
	}

	return listings, pagination, nil
}

// UpdateStatus updates the status of a listing (typically by an admin).
func (r *GORMRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) error {
	updates := map[string]interface{}{"status": status}
	// TODO: If adminNotes is a field on Listing model, add it to updates:
	// if adminNotes != nil { updates["admin_notes"] = *adminNotes }

	result := r.db.WithContext(ctx).Model(&Listing{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.ErrNotFound.WithDetails("Listing not found.")
	}
	return nil
}

func parseWKT(wkt string) (*PostGISPoint, error) {
	// Expected format: "POINT(-122.315804 47.615135)"
	wkt = strings.TrimSpace(wkt)

	if !strings.HasPrefix(wkt, "POINT(") || !strings.HasSuffix(wkt, ")") {
		return nil, fmt.Errorf("invalid WKT format: %s", wkt)
	}

	coords := strings.TrimPrefix(wkt, "POINT(")
	coords = strings.TrimSuffix(coords, ")")
	parts := strings.Fields(coords)

	if len(parts) != 2 {
		return nil, errors.New("invalid number of coordinates in POINT")
	}

	lon, err1 := strconv.ParseFloat(parts[0], 64)
	lat, err2 := strconv.ParseFloat(parts[1], 64)

	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("invalid coordinates: %v, %v", err1, err2)
	}

	return &PostGISPoint{
		Lon: lon,
		Lat: lat,
	}, nil
}

// FindExpiredListings retrieves listings whose expires_at is in the past and status is not 'expired'.
func (r *GORMRepository) FindExpiredListings(ctx context.Context, now time.Time) ([]Listing, error) {
	var listings []Listing
	err := r.db.WithContext(ctx).
		Where("expires_at <= ? AND status != ?", now, StatusExpired).
		Find(&listings).Error
	return listings, err
}

// CountListingsByUserIDAndStatus counts listings for a user with a specific status.
func (r *GORMRepository) CountListingsByUserIDAndStatus(ctx context.Context, userID uuid.UUID, status ListingStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Listing{}).Where("user_id = ? AND status = ?", userID, status).Count(&count).Error
	return count, err
}

// CountListingsByUserID counts all listings for a user, regardless of status.
func (r *GORMRepository) CountListingsByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Listing{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// GetRecentListings retrieves recent, active, non-event listings.
func (r *GORMRepository) GetRecentListings(ctx context.Context, page, pageSize int, currentUserID *uuid.UUID) ([]Listing, *common.Pagination, error) {
	var listings []Listing
	var total int64

	// Base query for recent listings
	query := r.db.WithContext(ctx).Model(&Listing{}).
		Joins("JOIN categories ON categories.id = listings.category_id").
		Where("categories.slug != ?", "events"). // Exclude events
		Where("listings.status = ?", StatusActive).
		Where("listings.is_admin_approved = ?", true).
		Where("listings.expires_at > ?", time.Now())

	// Count total records that match the criteria for pagination
	countQuery := query // Create a new query object for count to avoid issues with Order, Limit, Offset
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, nil, fmt.Errorf("counting recent listings failed: %w", err)
	}

	// Create pagination object
	// Note: common.NewPagination expects totalItems as int64, page and pageSize as int.
	// It calculates offset internally.
	pagination := common.NewPagination(total, page, pageSize)


	// Select query with preloads, ordering, limit, and offset
	// The Offset for GORM is calculated as (page - 1) * pageSize.
	// The common.Pagination.Offset() method should provide this.
	// If common.Pagination doesn't have an Offset() method, calculate it as:
	// offset := (pagination.CurrentPage - 1) * pagination.PageSize
	// Ensure common.NewPagination correctly sets CurrentPage and PageSize.
	offset := (page - 1) * pageSize
	if page <= 0 { // Ensure page is at least 1 for offset calculation
		offset = 0
	}


	err := query.Order("listings.created_at DESC").
		Limit(pageSize). // Use pageSize directly as Limit
		Offset(offset). // Use calculated offset
		Preload("User").
		Preload("Category").
		Preload("SubCategory").
		Preload("BabysittingDetails").
		Preload("HousingDetails").
		// Preload("EventDetails"). // EventDetails are not expected for non-event listings
		Find(&listings).Error

	if err != nil {
		return nil, nil, fmt.Errorf("fetching recent listings failed: %w", err)
	}

	// The pagination object should be returned based on the total count and input page/pageSize.
	// common.NewPagination should handle setting HasNext, HasPrev, etc.
	return listings, pagination, nil
}

// GetUpcomingEvents retrieves upcoming event listings.
func (r *GORMRepository) GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]Listing, *common.Pagination, error) {
	var listings []Listing
	var total int64

	now := time.Now()
	// Format date and time for SQL comparison.
	// It's generally better to pass time.Time objects directly to GORM if the DB driver supports it well,
	// as it avoids timezone issues and formatting complexities.
	// However, if specific string formats are needed for WHERE clauses:
	currentDate := now.Format("2006-01-02") // YYYY-MM-DD
	currentTime := now.Format("15:04:05")   // HH:MM:SS

	// Base query for upcoming events
	query := r.db.WithContext(ctx).Model(&Listing{}).
		Joins("JOIN categories ON categories.id = listings.category_id").
		Joins("JOIN listing_details_events ON listing_details_events.listing_id = listings.id"). // Ensure EventDetails exist
		Where("categories.slug = ?", "events").                                                  // Filter by event category slug
		Where("listings.status = ?", StatusActive).                                              // Active listings
		Where("listings.is_admin_approved = ?", true).                                           // Admin approved
		Where("listings.expires_at > ?", now).                                                   // Not expired
		Where("(listing_details_events.event_date > ?) OR (listing_details_events.event_date = ? AND (listing_details_events.event_time IS NULL OR listing_details_events.event_time >= ?))", currentDate, currentDate, currentTime) // Future or current day future time

	// Count total records for pagination
	// Important: Use a separate query for count without Order, Limit, Offset
	countQuery := r.db.WithContext(ctx).Model(&Listing{}).
		Joins("JOIN categories ON categories.id = listings.category_id").
		Joins("JOIN listing_details_events ON listing_details_events.listing_id = listings.id").
		Where("categories.slug = ?", "events").
		Where("listings.status = ?", StatusActive).
		Where("listings.is_admin_approved = ?", true).
		Where("listings.expires_at > ?", now).
		Where("(listing_details_events.event_date > ?) OR (listing_details_events.event_date = ? AND (listing_details_events.event_time IS NULL OR listing_details_events.event_time >= ?))", currentDate, currentDate, currentTime)

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, nil, fmt.Errorf("counting upcoming events failed: %w", err)
	}

	// Create pagination object
	pagination := common.NewPagination(total, page, pageSize)
	// Calculate offset. GORM uses 0-based offset.
	offset := (page - 1) * pageSize
	if page <= 0 { // Ensure page is at least 1 for offset calculation
		offset = 0
	}


	// Select query with preloads, ordering, limit, and offset
	err := query.Order("listing_details_events.event_date ASC, listing_details_events.event_time ASC").
		Limit(pageSize).
		Offset(offset).
		Preload("User").           // User who created the event
		Preload("Category").       // Event category details
		Preload("SubCategory").    // In case events start using subcategories
		Preload("EventDetails").   // Essential for event date, time, etc.
		// BabysittingDetails and HousingDetails are not relevant for events, so not preloaded.
		Find(&listings).Error

	if err != nil {
		return nil, nil, fmt.Errorf("fetching upcoming events failed: %w", err)
	}

	return listings, pagination, nil
}
