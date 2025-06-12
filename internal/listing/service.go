// File: internal/listing/service.go
package listing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/notification"
	"seattle_info_backend/internal/listing/esutil" // Added for ListingToElasticsearchDoc
	platformElasticsearch "seattle_info_backend/internal/platform/elasticsearch"
	"seattle_info_backend/internal/user"

	"encoding/json" // Keep for ES search result processing
	"strings"       // Keep for ES search body and IndexRequest

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/google/uuid"
	"go.uber.org/zap"
)


// Service defines the interface for listing-related business logic.
type Service interface {
	CreateListing(ctx context.Context, userID uuid.UUID, req CreateListingRequest) (*Listing, error)
	GetListingByID(ctx context.Context, id uuid.UUID, authenticatedUserID *uuid.UUID) (*Listing, error)
	UpdateListing(ctx context.Context, id uuid.UUID, userID uuid.UUID, req UpdateListingRequest) (*Listing, error)
	DeleteListing(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
	SearchListings(ctx context.Context, query ListingSearchQuery, authenticatedUserID *uuid.UUID) ([]Listing, *common.Pagination, error)
	GetUserListings(ctx context.Context, userID uuid.UUID, query UserListingsQuery) ([]Listing, *common.Pagination, error)
	GetRecentListings(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error)
	GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error)

	// Admin specific
	AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) (*Listing, error)
	AdminApproveListing(ctx context.Context, id uuid.UUID) (*Listing, error)
	AdminGetListingByID(ctx context.Context, id uuid.UUID) (*Listing, error)

	// Jobs related (can be called by cron jobs)
	ExpireListings(ctx context.Context) (int, error)
}

// ServiceImplementation implements the listing Service interface.
type ServiceImplementation struct {
	repo                Repository
	userRepo            user.Repository
	categoryService     category.Service
	notificationService notification.Service
	cfg                 *config.Config
	logger              *zap.Logger
	esClient            *platformElasticsearch.ESClientWrapper // Re-add field
}

// NewService creates a new listing service.
func NewService(
	repo Repository,
	userRepo user.Repository,
	categoryService category.Service,
	notificationService notification.Service,
	cfg *config.Config,
	logger *zap.Logger,
	esClient *platformElasticsearch.ESClientWrapper, // Re-add param
) Service {
	return &ServiceImplementation{
		repo:                repo,
		userRepo:            userRepo,
		categoryService:     categoryService,
		notificationService: notificationService,
		cfg:                 cfg,
		logger:              logger,
		esClient:            esClient, // Re-add assignment
	}
}

// CreateListing handles the business logic for creating a new listing.
func (s *ServiceImplementation) CreateListing(ctx context.Context, userID uuid.UUID, req CreateListingRequest) (*Listing, error) {
	// Ensure ES client is available before proceeding with DB operations if indexing is critical path.
	// For eventual consistency, we can proceed and log ES errors.
	if s.esClient == nil || s.esClient.Client == nil {
		s.logger.Warn("Elasticsearch client not available, will not index new listing")
	}

	cat, err := s.categoryService.GetCategoryByID(ctx, req.CategoryID, true)
	if err != nil {
		s.logger.Warn("Invalid category ID during listing creation", zap.String("categoryID", req.CategoryID.String()), zap.Error(err))
		return nil, common.ErrBadRequest.WithDetails("Invalid category ID provided.")
	}
	if req.SubCategoryID != nil && *req.SubCategoryID != uuid.Nil {
		foundSubCat := false
		for _, sc := range cat.SubCategories {
			if sc.ID == *req.SubCategoryID {
				foundSubCat = true
				break
			}
		}
		if !foundSubCat {
			s.logger.Warn("Invalid subcategory ID for the given category",
				zap.String("categoryID", req.CategoryID.String()),
				zap.String("subCategoryID", req.SubCategoryID.String()))
			return nil, common.ErrBadRequest.WithDetails("Subcategory does not belong to the specified category.")
		}
	} else if cat.Name == "Businesses" && (req.SubCategoryID == nil || *req.SubCategoryID == uuid.Nil) {
		return nil, common.ErrBadRequest.WithDetails("Subcategory is required for 'Business' listings.")
	}

	switch cat.Slug {
	case "baby-sitting":
		if req.BabysittingDetails == nil || len(req.BabysittingDetails.LanguagesSpoken) == 0 {
			return nil, common.ErrBadRequest.WithDetails("Languages spoken are required for Baby Sitting listings.")
		}
	case "housing":
		if req.HousingDetails == nil {
			return nil, common.ErrBadRequest.WithDetails("Housing details (property type) are required for Housing listings.")
		}
		if req.HousingDetails.PropertyType == HousingForRent && (req.HousingDetails.RentDetails == nil || *req.HousingDetails.RentDetails == "") {
			return nil, common.ErrBadRequest.WithDetails("Rent details are required for 'Property for Rent' housing listings.")
		}
		if req.HousingDetails.PropertyType == HousingForSale && (req.HousingDetails.SalePrice == nil || *req.HousingDetails.SalePrice <= 0) {
			return nil, common.ErrBadRequest.WithDetails("A valid sale price is required for 'Property for Sale' housing listings.")
		}
	case "events":
		if req.EventDetails == nil {
			return nil, common.ErrBadRequest.WithDetails("Event details (date) are required for Event listings.")
		}
	}

	postingUser, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("User not found when creating listing", zap.String("userID", userID.String()), zap.Error(err))
		return nil, common.ErrInternalServer.WithDetails("Could not retrieve user details.")
	}

	listingStatus := StatusActive
	isAdminApproved := true

	firstPostModelActiveUntil, err := s.getPlatformConfigDate("FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL")
	isFirstPostModelActive := false
	if err == nil && time.Now().Before(*firstPostModelActiveUntil) {
		isFirstPostModelActive = true
	} else if err != nil {
		s.logger.Warn("Could not parse FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL, assuming model is not active", zap.Error(err))
	}

	if isFirstPostModelActive && !postingUser.IsFirstPostApproved {
		userPostCount, err := s.repo.CountListingsByUserID(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to count user listings for first post check", zap.Error(err), zap.String("userID", userID.String()))
			return nil, common.ErrInternalServer.WithDetails("Could not verify posting eligibility.")
		}

		if userPostCount == 0 {
			listingStatus = StatusPendingApproval
			isAdminApproved = false
			s.logger.Info("First post by user, marking for admin approval", zap.String("userID", userID.String()))
		}
		if userPostCount > 0 {
			s.logger.Warn("User attempting to submit multiple posts before first approval", zap.String("userID", userID.String()))
			return nil, common.ErrForbidden.WithDetails("You must wait for your first post to be approved before submitting another.")
		}
	}

	lifespanDays := s.cfg.DefaultListingLifespanDays
	configLifespan, err := s.getPlatformConfigInt("DEFAULT_LISTING_LIFESPAN_DAYS")
	if err == nil && configLifespan > 0 {
		lifespanDays = configLifespan
	} else if err != nil {
		s.logger.Warn("Could not parse DEFAULT_LISTING_LIFESPAN_DAYS from app_configurations, using default from .env", zap.Error(err))
	}
	expiresAt := time.Now().AddDate(0, 0, lifespanDays)

	newListing := &Listing{
		UserID:          userID,
		CategoryID:      req.CategoryID,
		SubCategoryID:   req.SubCategoryID,
		Title:           req.Title,
		Description:     req.Description,
		Status:          listingStatus,
		ContactName:     req.ContactName,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		AddressLine1:    req.AddressLine1,
		AddressLine2:    req.AddressLine2,
		City:            req.City,
		State:           req.State,
		ZipCode:         req.ZipCode,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		ExpiresAt:       expiresAt,
		IsAdminApproved: isAdminApproved,
	}
	if req.Latitude != nil && req.Longitude != nil {
		newListing.Location = &PostGISPoint{Lat: *req.Latitude, Lon: *req.Longitude}
	}

	if req.BabysittingDetails != nil {
		newListing.BabysittingDetails = &ListingDetailsBabysitting{
			LanguagesSpoken: req.BabysittingDetails.LanguagesSpoken,
		}
	}
	if req.HousingDetails != nil {
		newListing.HousingDetails = &ListingDetailsHousing{
			PropertyType: req.HousingDetails.PropertyType,
			RentDetails:  req.HousingDetails.RentDetails,
			SalePrice:    req.HousingDetails.SalePrice,
		}
	}
	if req.EventDetails != nil {
		eventDate, _ := time.Parse("2006-01-02", req.EventDetails.EventDate)
		newListing.EventDetails = &ListingDetailsEvents{
			EventDate:     eventDate,
			EventTime:     req.EventDetails.EventTime,
			OrganizerName: req.EventDetails.OrganizerName,
			VenueName:     req.EventDetails.VenueName,
		}
	}

	if err := s.repo.Create(ctx, newListing); err != nil {
		s.logger.Error("Failed to create listing in repository", zap.Error(err))
		return nil, err
	}

	createdListing, err := s.repo.FindByID(ctx, newListing.ID, true)
	if err != nil {
		s.logger.Error("Failed to reload created listing with associations", zap.String("listingID", newListing.ID.String()), zap.Error(err))
		return newListing, nil
	}

	s.logger.Info("Listing created successfully", zap.String("listingID", createdListing.ID.String()), zap.String("status", string(createdListing.Status)))

	if s.notificationService != nil {
		var notifType notification.NotificationType
		var notifMessage string

		if createdListing.Status == StatusPendingApproval || !createdListing.IsAdminApproved {
			notifType = notification.ListingCreatedPendingApproval
			notifMessage = fmt.Sprintf("Your listing '%s' has been submitted and is pending review.", createdListing.Title)
		} else {
			notifType = notification.ListingCreatedLive
			notifMessage = fmt.Sprintf("Your listing '%s' has been successfully created and is now live!", createdListing.Title)
		}

		_, errNotif := s.notificationService.CreateNotification(ctx, createdListing.UserID, notifType, notifMessage, &createdListing.ID)
		if errNotif != nil {
			s.logger.Error("Failed to send listing creation notification",
				zap.Error(errNotif),
				zap.String("listingID", createdListing.ID.String()),
				zap.String("userID", createdListing.UserID.String()),
			)
		}
	}

	// Index to Elasticsearch
	if s.esClient != nil && s.esClient.Client != nil && createdListing != nil {
		docJSON, errDoc := esutil.ListingToElasticsearchDoc(createdListing) // Use esutil
		if errDoc != nil {
			s.logger.Error("Failed to convert listing to Elasticsearch document for CreateListing",
				zap.String("listingID", createdListing.ID.String()),
				zap.Error(errDoc),
			)
			// Don't return error, just log. DB transaction succeeded.
		} else {
			req := esapi.IndexRequest{
				Index:      platformElasticsearch.ListingsIndexName,
				DocumentID: createdListing.ID.String(),
				Body:       strings.NewReader(docJSON),
				Refresh:    "true", // Or "wait_for" or "false"
			}
			res, errIdx := req.Do(context.Background(), s.esClient.Client)
			if errIdx != nil {
				s.logger.Error("Failed to index listing in Elasticsearch for CreateListing",
					zap.String("listingID", createdListing.ID.String()),
					zap.Error(errIdx),
				)
			} else {
				defer res.Body.Close()
				if res.IsError() {
					s.logger.Error("Failed to index listing in Elasticsearch, response error for CreateListing",
						zap.String("listingID", createdListing.ID.String()),
						zap.String("status", res.Status()),
					)
				} else {
					s.logger.Info("Successfully indexed new listing in Elasticsearch", zap.String("listingID", createdListing.ID.String()))
				}
			}
		}
	}

	return createdListing, nil
}

// GetListingByID retrieves a listing by its ID, handling visibility rules.
func (s *ServiceImplementation) GetListingByID(ctx context.Context, id uuid.UUID, authenticatedUserID *uuid.UUID) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, true)
	if err != nil {
		return nil, err
	}

	if listing.Status == StatusPendingApproval {
		isOwner := authenticatedUserID != nil && listing.UserID == *authenticatedUserID
		if !isOwner {
			s.logger.Warn("Attempt to view pending listing by non-owner/non-admin",
				zap.String("listingID", id.String()),
				zap.Any("viewerID", authenticatedUserID),
			)
			return nil, common.ErrNotFound.WithDetails("Listing not found or access denied.")
		}
	}

	if listing.Status == StatusExpired && (authenticatedUserID == nil || listing.UserID != *authenticatedUserID) {
		return nil, common.ErrNotFound.WithDetails("Listing not found or has expired.")
	}

	return listing, nil
}

// AdminGetListingByID retrieves a listing by ID for admin purposes, bypassing some visibility rules.
func (s *ServiceImplementation) AdminGetListingByID(ctx context.Context, id uuid.UUID) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, true)
	if err != nil {
		return nil, err
	}
	return listing, nil
}

// UpdateListing handles the logic for updating an existing listing.
func (s *ServiceImplementation) UpdateListing(ctx context.Context, id uuid.UUID, userID uuid.UUID, req UpdateListingRequest) (*Listing, error) {
	existingListing, err := s.repo.FindByID(ctx, id, true)
	if err != nil {
		return nil, err
	}

	if existingListing.UserID != userID {
		s.logger.Warn("User attempted to update a listing they do not own",
			zap.String("listingID", id.String()),
			zap.String("editorUserID", userID.String()),
			zap.String("ownerUserID", existingListing.UserID.String()))
		return nil, common.ErrForbidden.WithDetails("You do not have permission to update this listing.")
	}

	if req.CategoryID != nil && *req.CategoryID != existingListing.CategoryID {
		return nil, common.ErrBadRequest.WithDetails("Changing the main category of a listing is not allowed. Please create a new listing.")
	}
	if req.SubCategoryID != nil {
		cat, errCat := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, true)
		if errCat != nil {
			return nil, common.ErrInternalServer.WithDetails("Could not verify category for subcategory update.")
		}
		foundSubCat := false
		for _, sc := range cat.SubCategories {
			if sc.ID == *req.SubCategoryID {
				foundSubCat = true
				break
			}
		}
		if !foundSubCat {
			return nil, common.ErrBadRequest.WithDetails("New subcategory does not belong to the listing's main category.")
		}
		existingListing.SubCategoryID = req.SubCategoryID
	} else if req.SubCategoryID == nil && existingListing.SubCategoryID != nil {
		currentCategory, _ := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
		if currentCategory != nil && currentCategory.Slug == "businesses" { // Assuming slug for "Business" is "businesses"
			return nil, common.ErrBadRequest.WithDetails("Cannot remove subcategory from a 'Business' listing.")
		}
		existingListing.SubCategoryID = nil
	}

	if req.Title != nil {
		existingListing.Title = *req.Title
	}
	if req.Description != nil {
		existingListing.Description = *req.Description
	}
	if req.ContactName != nil {
		existingListing.ContactName = req.ContactName
	}
	if req.ContactEmail != nil {
		existingListing.ContactEmail = req.ContactEmail
	}
	if req.ContactPhone != nil {
		existingListing.ContactPhone = req.ContactPhone
	}
	if req.AddressLine1 != nil {
		existingListing.AddressLine1 = req.AddressLine1
	}
	if req.AddressLine2 != nil {
		existingListing.AddressLine2 = req.AddressLine2
	}
	if req.City != nil {
		existingListing.City = req.City
	}
	if req.State != nil {
		existingListing.State = req.State
	}
	if req.ZipCode != nil {
		existingListing.ZipCode = req.ZipCode
	}

	locationChanged := false
	if req.Latitude != nil {
		existingListing.Latitude = req.Latitude
		locationChanged = true
	}
	if req.Longitude != nil {
		existingListing.Longitude = req.Longitude
		locationChanged = true
	}
	if locationChanged && existingListing.Latitude != nil && existingListing.Longitude != nil {
		existingListing.Location = &PostGISPoint{Lat: *existingListing.Latitude, Lon: *existingListing.Longitude}
	} else if locationChanged && (existingListing.Latitude == nil || existingListing.Longitude == nil) {
		existingListing.Location = nil
		existingListing.Latitude = nil
		existingListing.Longitude = nil
	}

	if existingListing.Category.Slug == "" {
		cat, catErr := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
		if catErr != nil {
			s.logger.Error("Failed to retrieve category for listing update",
				zap.String("listingID", id.String()),
				zap.String("categoryID", existingListing.CategoryID.String()),
				zap.Error(catErr))
			return nil, common.ErrInternalServer.WithDetails("Could not verify listing category for update.")
		}
		existingListing.Category = *cat
	}

	if existingListing.Category.Slug != "" {
		switch existingListing.Category.Slug {
		case "baby-sitting":
			if req.BabysittingDetails != nil {
				if existingListing.BabysittingDetails == nil {
					existingListing.BabysittingDetails = &ListingDetailsBabysitting{ListingID: existingListing.ID}
				}
				existingListing.BabysittingDetails.LanguagesSpoken = req.BabysittingDetails.LanguagesSpoken
			}
		case "housing":
			if req.HousingDetails != nil {
				if existingListing.HousingDetails == nil {
					existingListing.HousingDetails = &ListingDetailsHousing{ListingID: existingListing.ID}
				}
				existingListing.HousingDetails.PropertyType = req.HousingDetails.PropertyType
				if req.HousingDetails.RentDetails != nil {
					existingListing.HousingDetails.RentDetails = req.HousingDetails.RentDetails
				}
				if req.HousingDetails.SalePrice != nil {
					existingListing.HousingDetails.SalePrice = req.HousingDetails.SalePrice
				}
			}
		case "events":
			if req.EventDetails != nil {
				if existingListing.EventDetails == nil {
					existingListing.EventDetails = &ListingDetailsEvents{ListingID: existingListing.ID}
				}
				if req.EventDetails.EventDate != "" {
					eventDate, errDate := time.Parse("2006-01-02", req.EventDetails.EventDate)
					if errDate != nil {
						s.logger.Warn("Invalid event_date format during listing update",
							zap.String("listingID", id.String()),
							zap.String("eventDate", req.EventDetails.EventDate),
							zap.Error(errDate))
					} else {
						existingListing.EventDetails.EventDate = eventDate
					}
				}
				if req.EventDetails.EventTime != nil {
					existingListing.EventDetails.EventTime = req.EventDetails.EventTime
				}
				if req.EventDetails.OrganizerName != nil {
					existingListing.EventDetails.OrganizerName = req.EventDetails.OrganizerName
				}
				if req.EventDetails.VenueName != nil {
					existingListing.EventDetails.VenueName = req.EventDetails.VenueName
				}
			}
		}
	}

	if existingListing.Status == StatusRejected || existingListing.Status == StatusAdminRemoved {
		// Business logic for re-approval or state change on edit can be added here.
	}

	if err := s.repo.Update(ctx, existingListing); err != nil {
		s.logger.Error("Failed to update listing in repository", zap.Error(err), zap.String("listingID", id.String()))
		return nil, err
	}

	updatedListing, err := s.repo.FindByID(ctx, existingListing.ID, true)
	if err != nil {
		s.logger.Error("Failed to reload updated listing with associations", zap.String("listingID", existingListing.ID.String()), zap.Error(err))
		return existingListing, nil
	}

	s.logger.Info("Listing updated successfully", zap.String("listingID", updatedListing.ID.String()))

	// Update Elasticsearch index
	if s.esClient != nil && s.esClient.Client != nil && updatedListing != nil {
		docJSON, errDoc := esutil.ListingToElasticsearchDoc(updatedListing) // Use esutil
		if errDoc != nil {
			s.logger.Error("Failed to convert listing to Elasticsearch document for UpdateListing",
				zap.String("listingID", updatedListing.ID.String()),
				zap.Error(errDoc),
			)
		} else {
			req := esapi.IndexRequest{
				Index:      platformElasticsearch.ListingsIndexName,
				DocumentID: updatedListing.ID.String(),
				Body:       strings.NewReader(docJSON),
				Refresh:    "true",
			}
			res, errIdx := req.Do(context.Background(), s.esClient.Client)
			if errIdx != nil {
				s.logger.Error("Failed to update listing in Elasticsearch for UpdateListing",
					zap.String("listingID", updatedListing.ID.String()),
					zap.Error(errIdx),
				)
			} else {
				defer res.Body.Close()
				if res.IsError() {
					s.logger.Error("Failed to update listing in Elasticsearch, response error for UpdateListing",
						zap.String("listingID", updatedListing.ID.String()),
						zap.String("status", res.Status()),
					)
				} else {
					s.logger.Info("Successfully updated listing in Elasticsearch", zap.String("listingID", updatedListing.ID.String()))
				}
			}
		}
	}
	return updatedListing, nil
}

// DeleteListing handles deleting a listing.
func (s *ServiceImplementation) DeleteListing(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	// Check if ES client is available before proceeding with DB operations if indexing is critical.
	// For eventual consistency, we can proceed and log ES errors.
	if s.esClient == nil || s.esClient.Client == nil {
		s.logger.Warn("Elasticsearch client not available, will not attempt to delete listing from index")
	}

	if err := s.repo.Delete(ctx, id, userID); err != nil {
		s.logger.Error("Failed to delete listing from database", zap.Error(err), zap.String("listingID", id.String()), zap.String("userID", userID.String()))
		return err
	}
	s.logger.Info("Listing deleted successfully from database", zap.String("listingID", id.String()), zap.String("userID", userID.String()))

	// Delete from Elasticsearch
	if s.esClient != nil && s.esClient.Client != nil {
		req := esapi.DeleteRequest{
			Index:      platformElasticsearch.ListingsIndexName,
			DocumentID: id.String(),
			Refresh:    "true",
		}
		res, errIdx := req.Do(context.Background(), s.esClient.Client)
		if errIdx != nil {
			s.logger.Error("Failed to delete listing from Elasticsearch for DeleteListing",
				zap.String("listingID", id.String()),
				zap.Error(errIdx),
			)
			// Do not return error, DB deletion was successful.
		} else {
			defer res.Body.Close()
			if res.IsError() && res.StatusCode != 404 { // 404 means already deleted or never existed, which is fine.
				s.logger.Error("Failed to delete listing from Elasticsearch, response error for DeleteListing",
					zap.String("listingID", id.String()),
					zap.String("status", res.Status()),
				)
			} else if res.StatusCode == 404 {
				s.logger.Info("Listing not found in Elasticsearch for deletion (or already deleted)", zap.String("listingID", id.String()))
			} else {
				s.logger.Info("Successfully deleted listing from Elasticsearch", zap.String("listingID", id.String()))
			}
		}
	}
	return nil
}

// SearchListings performs a search for listings based on various criteria using Elasticsearch.
func (s *ServiceImplementation) SearchListings(ctx context.Context, queryParams ListingSearchQuery, authenticatedUserID *uuid.UUID) ([]Listing, *common.Pagination, error) {
	if s.esClient == nil || s.esClient.Client == nil {
		s.logger.Warn("Elasticsearch client not available, falling back to DB search for SearchListings")
		// Fallback to original DB search if ES is not available.
		// This part of the original function can be extracted or duplicated if needed.
		// For this refactoring, we'll assume ES is primary. If it must fallback, that's an extra step.
		// return s.searchListingsDB(ctx, queryParams, authenticatedUserID)
		return nil, nil, common.ErrServiceUnavailable.WithDetails("Search service temporarily unavailable.")
	}

	esQuery := map[string]interface{}{}
	boolQuery := map[string]interface{}{}
	mustClauses := []map[string]interface{}{}
	filterClauses := []map[string]interface{}{}

	// Text Search (`q`)
	if queryParams.SearchTerm != nil && *queryParams.SearchTerm != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  *queryParams.SearchTerm,
				"fields": []string{"title", "description", "contact_name", "address_line1"}, // Add more searchable fields
			},
		})
	}

	// Filters
	if queryParams.CategoryID != nil && *queryParams.CategoryID != uuid.Nil {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"category_id": queryParams.CategoryID.String()},
		})
	}
	if queryParams.SubCategoryID != nil && *queryParams.SubCategoryID != uuid.Nil {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"sub_category_id": queryParams.SubCategoryID.String()},
		})
	}
	if queryParams.UserID != nil && *queryParams.UserID != uuid.Nil {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"user_id": queryParams.UserID.String()},
		})
	}

	// Status and Expiry
	if queryParams.Status != "" {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"status": string(queryParams.Status)},
		})
	} else {
		// Default: only active, non-expired listings unless IncludeExpired is true
		if !queryParams.IncludeExpired {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{"status": []string{string(StatusActive)}}, // Only active if not specified
			})
			filterClauses = append(filterClauses, map[string]interface{}{
				"range": map[string]interface{}{
					"expires_at": map[string]interface{}{"gt": "now/m"}, // Use date math, round to the minute
				},
			})
		}
	}
	// is_admin_approved filter (typically true for public searches)
	if queryParams.IsAdminApproved != nil {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"is_admin_approved": *queryParams.IsAdminApproved},
		})
	} else {
		// Default to only showing admin-approved listings for public searches
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"is_admin_approved": true},
		})
	}


	// Geo Search
	if queryParams.Latitude != nil && queryParams.Longitude != nil && queryParams.MaxDistanceKM != nil && *queryParams.MaxDistanceKM > 0 {
		filterClauses = append(filterClauses, map[string]interface{}{
			"geo_distance": map[string]interface{}{
				"distance": fmt.Sprintf("%fkm", *queryParams.MaxDistanceKM),
				"location": map[string]float64{
					"lat": *queryParams.Latitude,
					"lon": *queryParams.Longitude,
				},
			},
		})
	}

	if len(mustClauses) > 0 {
		boolQuery["must"] = mustClauses
	}
	if len(filterClauses) > 0 {
		boolQuery["filter"] = filterClauses
	}
	esQuery["query"] = map[string]interface{}{"bool": boolQuery}

	// Sorting
	var sorters []map[string]interface{}
	sortOrder := "asc"
	if queryParams.SortOrder != "" {
		sortOrder = strings.ToLower(queryParams.SortOrder)
	}

	if queryParams.SortBy == "distance" && queryParams.Latitude != nil && queryParams.Longitude != nil {
		sorters = append(sorters, map[string]interface{}{
			"_geo_distance": map[string]interface{}{
				"location": map[string]interface{}{
					"lat": *queryParams.Latitude,
					"lon": *queryParams.Longitude,
				},
				"order":         sortOrder,
				"unit":          "km",
				"distance_type": "arc", // More accurate
			},
		})
	} else if queryParams.SortBy != "" {
		// Map API sort fields to ES fields if necessary
		esSortField := queryParams.SortBy
		if esSortField == "created_at" || esSortField == "expires_at" { // Example, add others if needed
			// Default order for date fields often 'desc'
			if queryParams.SortOrder == "" { // if user didn't specify order
				sortOrder = "desc"
			}
		}
		sorters = append(sorters, map[string]interface{}{
			esSortField: map[string]interface{}{"order": sortOrder},
		})
	} else {
		// Default sort: by score if there's a text query, otherwise by creation date
		if queryParams.SearchTerm != nil && *queryParams.SearchTerm != "" {
			// default is _score desc
		} else {
			sorters = append(sorters, map[string]interface{}{
				"created_at": map[string]interface{}{"order": "desc"},
			})
		}
	}
	if len(sorters) > 0 {
		esQuery["sort"] = sorters
	}

	// Pagination
	from := (queryParams.Page - 1) * queryParams.PageSize
	esQuery["from"] = from
	esQuery["size"] = queryParams.PageSize

	// Execute Search
	queryJSON, err := json.Marshal(esQuery)
	if err != nil {
		s.logger.Error("Failed to marshal Elasticsearch query", zap.Error(err), zap.Any("query", esQuery))
		return nil, nil, common.ErrInternalServer.WithDetails("Error building search query.")
	}
	s.logger.Debug("Elasticsearch query", zap.String("query", string(queryJSON)))

	esRes, err := s.esClient.Client.Search(
		s.esClient.Client.Search.WithContext(ctx),
		s.esClient.Client.Search.WithIndex(platformElasticsearch.ListingsIndexName),
		s.esClient.Client.Search.WithBody(strings.NewReader(string(queryJSON))),
		s.esClient.Client.Search.WithTrackTotalHits(true),
		s.esClient.Client.Search.WithErrorTrace(s.cfg.GinMode == "debug"), // Enable error trace for debug
	)
	if err != nil {
		s.logger.Error("Error executing Elasticsearch search", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Error executing search.")
	}
	defer esRes.Body.Close()

	if esRes.IsError() {
		var errBody map[string]interface{}
		if json.NewDecoder(esRes.Body).Decode(&errBody) == nil {
			s.logger.Error("Elasticsearch search returned error", zap.String("status", esRes.Status()), zap.Any("error_body", errBody))
		} else {
			s.logger.Error("Elasticsearch search returned error, could not parse body", zap.String("status", esRes.Status()))
		}
		return nil, nil, common.ErrInternalServer.WithDetails("Search operation failed.")
	}

	// Process Results
	var esSearchResults struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string          `json:"_id"`
				Source json.RawMessage `json:"_source"` // Use RawMessage to defer parsing
				Sort   []interface{}   `json:"sort,omitempty"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(esRes.Body).Decode(&esSearchResults); err != nil {
		s.logger.Error("Failed to decode Elasticsearch search results", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Error processing search results.")
	}

	listings := make([]Listing, 0, len(esSearchResults.Hits.Hits))
	for _, hit := range esSearchResults.Hits.Hits {
		var listing Listing
		// Attempt to unmarshal _source into the Listing struct
		// This assumes listingToElasticsearchDoc indexed fields compatible with the Listing struct
		if err := json.Unmarshal(hit.Source, &listing); err != nil {
			s.logger.Error("Failed to unmarshal listing from ES _source", zap.String("listingID", hit.ID), zap.Error(err))
			continue // Skip this problematic document
		}
		listing.ID, _ = uuid.Parse(hit.ID) // _id is the UUID

		// TODO: Populate Listing.Distance if geo_sort was applied (from hit.Sort)
		// TODO: Populate Category, SubCategory, User by fetching from DB if not fully denormalized in ES
		// For now, these will be zero/nil as they are not directly in _source as complex types.
		// This highlights the decision point between Option A and Option B.
		// For a true Listing object, we'd need to hydrate these associations.
		// If ListingResponse is the target, ensure all ListingResponse fields are in _source.

		listings = append(listings, listing)
	}

	pagination := &common.Pagination{
		CurrentPage: queryParams.Page,
		PageSize:    queryParams.PageSize,
		TotalItems:  esSearchResults.Hits.Total.Value,
		TotalPages:  (esSearchResults.Hits.Total.Value + queryParams.PageSize - 1) / queryParams.PageSize,
	}

	return listings, pagination, nil
}

// GetUserListings retrieves listings for a specific user.
func (s *ServiceImplementation) GetUserListings(ctx context.Context, userID uuid.UUID, query UserListingsQuery) ([]Listing, *common.Pagination, error) {
	listings, pagination, err := s.repo.FindByUserID(ctx, userID, query)
	if err != nil {
		s.logger.Error("Failed to get user listings from repository",
			zap.String("userID", userID.String()),
			zap.Any("query", query),
			zap.Error(err),
		)
		return nil, nil, err
	}

	s.logger.Debug("Successfully retrieved user listings",
		zap.String("userID", userID.String()),
		zap.Int("count", len(listings)),
	)
	return listings, pagination, nil
}

// AdminUpdateListingStatus handles admin updates to a listing's status.
func (s *ServiceImplementation) AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, newStatus ListingStatus, adminNotes *string) (*Listing, error) {
	listingBeforeUpdate, err := s.repo.FindByID(ctx, id, true) // Preload associations
	if err != nil {
		s.logger.Warn("AdminUpdateListingStatus: Listing not found before update", zap.String("listingID", id.String()), zap.Error(err))
		return nil, err
	}
	originalStatus := listingBeforeUpdate.Status
	originalIsAdminApproved := listingBeforeUpdate.IsAdminApproved

	userWasUpdated := false
	if newStatus == StatusActive && originalStatus == StatusPendingApproval && listingBeforeUpdate.User != nil && !listingBeforeUpdate.User.IsFirstPostApproved {
		postingUser := listingBeforeUpdate.User
		// It's safer to fetch the user again to ensure we have the latest state before updating
		fullUser, userErr := s.userRepo.FindByID(ctx, postingUser.ID)
		if userErr == nil {
			if !fullUser.IsFirstPostApproved {
				fullUser.IsFirstPostApproved = true
				if updateErr := s.userRepo.Update(ctx, fullUser); updateErr != nil {
					s.logger.Error("Failed to update user IsFirstPostApproved flag", zap.Error(updateErr), zap.String("userID", fullUser.ID.String()))
				} else {
					s.logger.Info("User's first post approved, flag updated", zap.String("userID", fullUser.ID.String()))
					userWasUpdated = true
				}
			}
		} else {
			s.logger.Error("Failed to find user to update IsFirstPostApproved flag", zap.Error(userErr), zap.String("userID", postingUser.ID.String()))
		}
	}

	// Update listing status
	if err := s.repo.UpdateStatus(ctx, id, newStatus, adminNotes); err != nil {
		s.logger.Error("Failed to admin update listing status in repo", zap.Error(err), zap.String("listingID", id.String()))
		return nil, err
	}

	// If status is now Active, ensure IsAdminApproved is true
	if newStatus == StatusActive {
		// Fetch the listing again to get the result of UpdateStatus
		tempListingForApprovalUpdate, findErr := s.repo.FindByID(ctx, id, false) // No need to preload here
		if findErr == nil {
			if !tempListingForApprovalUpdate.IsAdminApproved { // Only update if it's not already true
				tempListingForApprovalUpdate.IsAdminApproved = true
				// Use a more targeted update for IsAdminApproved rather than full Update.
				// This might require a specific repo method or careful use of Updates.
				// For now, using existing Update, but be mindful of its scope.
				if errUpdate := s.repo.Update(ctx, tempListingForApprovalUpdate); errUpdate != nil {
					s.logger.Error("Failed to explicitly set IsAdminApproved to true after status update", zap.Error(errUpdate), zap.String("listingID", id.String()))
				}
			}
		}
	}


	updatedListing, err := s.repo.FindByID(ctx, id, true)
	if err != nil {
		s.logger.Error("AdminUpdateListingStatus: Failed to reload listing after update", zap.String("listingID", id.String()), zap.Error(err))
		return nil, err
	}

	if s.notificationService != nil &&
		(originalStatus == StatusPendingApproval || !originalIsAdminApproved) &&
		updatedListing.Status == StatusActive && updatedListing.IsAdminApproved {

		notifType := notification.ListingApprovedLive
		notifMessage := fmt.Sprintf("Great news! Your listing '%s' has been approved and is now live.", updatedListing.Title)

		_, errNotif := s.notificationService.CreateNotification(ctx, updatedListing.UserID, notifType, notifMessage, &updatedListing.ID)
		if errNotif != nil {
			s.logger.Error("Failed to send listing approved notification",
				zap.Error(errNotif),
				zap.String("listingID", updatedListing.ID.String()),
				zap.String("userID", updatedListing.UserID.String()),
			)
		}
	}

	s.logger.Info("Admin updated listing status", zap.String("listingID", id.String()), zap.String("newStatus", string(newStatus)), zap.Bool("userFirstPostApprovedUpdated", userWasUpdated))
	return updatedListing, nil
}


// AdminApproveListing approves a listing.
func (s *ServiceImplementation) AdminApproveListing(ctx context.Context, id uuid.UUID) (*Listing, error) {
	return s.AdminUpdateListingStatus(ctx, id, StatusActive, nil)
}

// ExpireListings finds and marks overdue listings as expired.
func (s *ServiceImplementation) ExpireListings(ctx context.Context) (int, error) {
	now := time.Now()
	expiredListings, err := s.repo.FindExpiredListings(ctx, now)
	if err != nil {
		s.logger.Error("Failed to find expired listings", zap.Error(err))
		return 0, err
	}

	count := 0
	for _, listing := range expiredListings {
		listing.Status = StatusExpired
		if err := s.repo.UpdateStatus(ctx, listing.ID, StatusExpired, nil); err != nil {
			s.logger.Error("Failed to update listing to expired", zap.Error(err), zap.String("listingID", listing.ID.String()))
		} else {
			s.logger.Info("Listing expired and status updated", zap.String("listingID", listing.ID.String()))
			count++
		}
	}
	s.logger.Info("Listing expiry job completed", zap.Int("expired_count", count), zap.Int("found_to_expire", len(expiredListings)))
	return count, nil
}

func (s *ServiceImplementation) getPlatformConfigDate(key string) (*time.Time, error) {
	if key == "FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL" {
		activeMonths := s.cfg.FirstPostApprovalActiveMonths
		if activeMonths > 0 {
			val := time.Now().AddDate(0, activeMonths, 0)
			return &val, nil
		}
	}
	return nil, errors.New("config key not found or not a date: " + key)
}

func (s *ServiceImplementation) getPlatformConfigInt(key string) (int, error) {
	if key == "DEFAULT_LISTING_LIFESPAN_DAYS" {
		return s.cfg.DefaultListingLifespanDays, nil
	}
	if key == "MAX_LISTING_DISTANCE_KM" {
		return s.cfg.MaxListingDistanceKM, nil
	}
	return 0, errors.New("config key not found or not an int: " + key)
}

// GetRecentListings retrieves recent non-event listings.
func (s *ServiceImplementation) GetRecentListings(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error) {
	listings, pagination, err := s.repo.GetRecentListings(ctx, page, pageSize, nil)
	if err != nil {
		s.logger.Error("Failed to get recent listings from repository", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not retrieve recent listings.")
	}

	listingResponses := make([]ListingResponse, len(listings))
	for i, l := range listings {
		listingResponses[i] = ToListingResponse(&l, false)
	}

	return listingResponses, pagination, nil
}

// GetUpcomingEvents retrieves upcoming event listings.
func (s *ServiceImplementation) GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error) {
	listings, pagination, err := s.repo.GetUpcomingEvents(ctx, page, pageSize)
	if err != nil {
		s.logger.Error("Failed to get upcoming events from repository", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not retrieve upcoming events.")
	}

	listingResponses := make([]ListingResponse, len(listings))
	for i, l := range listings {
		listingResponses[i] = ToListingResponse(&l, false)
	}

	return listingResponses, pagination, nil
}
