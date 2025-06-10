// File: internal/listing/service.go
package listing

import (
	"context"
	"errors"
	"time"

	"fmt" // Added for notification messages
	"seattle_info_backend/internal/category" // For category validation
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/notification" // Add this
	"seattle_info_backend/internal/user"         // For user details, first post approval status

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

	// Admin specific
	AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) (*Listing, error)
	AdminApproveListing(ctx context.Context, id uuid.UUID) (*Listing, error)
	AdminGetListingByID(ctx context.Context, id uuid.UUID) (*Listing, error) // Bypasses some user checks

	// Jobs related (can be called by cron jobs)
	ExpireListings(ctx context.Context) (int, error)

	GetRecentListings(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error)
	GetUpcomingEvents(ctx context.Context, page, pageSize int) ([]ListingResponse, *common.Pagination, error)
}

// ServiceImplementation implements the listing Service interface.
type ServiceImplementation struct {
	repo                Repository
	userRepo            user.Repository  // To check user's first post status, etc.
	categoryService     category.Service // To validate category/subcategory IDs
	notificationService notification.Service
	cfg                 *config.Config
	logger              *zap.Logger
}

// NewService creates a new listing service.
func NewService(
	repo Repository,
	userRepo user.Repository,
	categoryService category.Service,
	notificationService notification.Service,
	cfg *config.Config,
	logger *zap.Logger,
) Service {
	return &ServiceImplementation{
		repo:                repo,
		userRepo:            userRepo,
		categoryService:     categoryService,
		notificationService: notificationService,
		cfg:                 cfg,
		logger:              logger,
	}
}

// CreateListing handles the business logic for creating a new listing.
func (s *ServiceImplementation) CreateListing(ctx context.Context, userID uuid.UUID, req CreateListingRequest) (*Listing, error) {
	// Validate Category and SubCategory
	cat, err := s.categoryService.GetCategoryByID(ctx, req.CategoryID, true) // Preload subcategories for validation
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
	} else if cat.Name == "Businesses" && (req.SubCategoryID == nil || *req.SubCategoryID == uuid.Nil) { // BR1.2 specific for "Business"
		// Assuming "Businesses" is the name. Better to use slug or a constant.
		// If the main category is "Business", a subcategory is mandatory.
		return nil, common.ErrBadRequest.WithDetails("Subcategory is required for 'Business' listings.")
	}

	// Validate category-specific details based on BR1
	// E.g., if category is "Baby Sitting", languages_spoken is required in BabysittingDetails (BR1.3)
	// This logic should be more robust, perhaps using category slugs.
	switch cat.Slug { // Assuming slugs are 'baby-sitting', 'housing', 'events'
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

	// Handle first post moderation (BR3.3)
	postingUser, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		s.logger.Error("User not found when creating listing", zap.String("userID", userID.String()), zap.Error(err))
		return nil, common.ErrInternalServer.WithDetails("Could not retrieve user details.")
	}

	listingStatus := StatusActive
	isAdminApproved := true // Default to true unless first post moderation applies

	// Check if first-post approval model is active
	firstPostModelActiveUntil, err := s.getPlatformConfigDate("FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL")
	isFirstPostModelActive := false
	if err == nil && time.Now().Before(*firstPostModelActiveUntil) {
		isFirstPostModelActive = true
	} else if err != nil {
		s.logger.Warn("Could not parse FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL, assuming model is not active", zap.Error(err))
	}

	if isFirstPostModelActive && !postingUser.IsFirstPostApproved {
		// Check if this user has any other posts (approved or pending)
		// This simplified check counts all posts. A more accurate check might look for *approved* posts.
		userPostCount, err := s.repo.CountListingsByUserID(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to count user listings for first post check", zap.Error(err), zap.String("userID", userID.String()))
			return nil, common.ErrInternalServer.WithDetails("Could not verify posting eligibility.")
		}

		if userPostCount == 0 { // This is their absolute first post submission
			listingStatus = StatusPendingApproval
			isAdminApproved = false
			s.logger.Info("First post by user, marking for admin approval", zap.String("userID", userID.String()))
		}
		// BR3.3: "During this initial 'first post pending approval' phase, the user is restricted to submitting only that single post."
		// If they already have a post (which must be pending if IsFirstPostApproved is false), they can't post another.
		if userPostCount > 0 { // This means they have a post, and since IsFirstPostApproved is false, it must be pending.
			// Or, they might have had a post rejected. A more granular status check on existing posts might be needed.
			// For now, if IsFirstPostApproved is false and they have any post, block.
			s.logger.Warn("User attempting to submit multiple posts before first approval", zap.String("userID", userID.String()))
			return nil, common.ErrForbidden.WithDetails("You must wait for your first post to be approved before submitting another.")
		}
	}

	// Listing lifespan (BR2.3)
	lifespanDays := s.cfg.DefaultListingLifespanDays // From .env via config struct
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

	// Populate details
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
		eventDate, _ := time.Parse("2006-01-02", req.EventDetails.EventDate) // Validation ensures format
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

	// Important: Reload the created listing with associations for the response
	createdListing, err := s.repo.FindByID(ctx, newListing.ID, true)
	if err != nil {
		s.logger.Error("Failed to reload created listing with associations", zap.String("listingID", newListing.ID.String()), zap.Error(err))
		// Return the original newListing without full associations if reload fails, or handle error
		return newListing, nil // Or return error
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
			// Do not fail the operation due to notification error
		}
	}
	return createdListing, nil
}

// GetListingByID retrieves a listing by its ID, handling visibility rules.
func (s *ServiceImplementation) GetListingByID(ctx context.Context, id uuid.UUID, authenticatedUserID *uuid.UUID) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, true) // Preload associations
	if err != nil {
		return nil, err // Repo returns common.ErrNotFound
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
		if currentCategory != nil && currentCategory.Slug == "businesses" {
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

<<<<<<< HEAD
	currentCat, _ := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
	if currentCat != nil {
		switch currentCat.Slug {
=======
	// Update specific details based on the listing's category.
	// The existingListing.Category should be preloaded by FindByID.
	if existingListing.Category.Slug == "" {
		// Attempt to load category if it wasn't preloaded or is missing for some reason.
		// This is a fallback, ideally Category is always preloaded with the listing.
		cat, catErr := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
		if catErr != nil {
			s.logger.Error("Failed to retrieve category for listing update",
				zap.String("listingID", id.String()),
				zap.String("categoryID", existingListing.CategoryID.String()),
				zap.Error(catErr))
			return nil, common.ErrInternalServer.WithDetails("Could not verify listing category for update.")
		}
		existingListing.Category = *cat // Update the category on the listing
	}

	// Use existingListing.Category.Slug for the switch
	if existingListing.Category.Slug != "" {
		switch existingListing.Category.Slug {
>>>>>>> origin/feat/user-listings-management
		case "baby-sitting":
			if req.BabysittingDetails != nil {
				if existingListing.BabysittingDetails == nil {
					existingListing.BabysittingDetails = &ListingDetailsBabysitting{ListingID: existingListing.ID}
				}
				// LanguagesSpoken is a slice, typically replaced entirely if provided.
				existingListing.BabysittingDetails.LanguagesSpoken = req.BabysittingDetails.LanguagesSpoken
			}
		case "housing":
			if req.HousingDetails != nil {
				if existingListing.HousingDetails == nil {
					existingListing.HousingDetails = &ListingDetailsHousing{ListingID: existingListing.ID}
				}
				// PropertyType is required in CreateListingHousingDetailsRequest,
				// so if req.HousingDetails is not nil, PropertyType should be valid.
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
				// EventDate is required in CreateListingEventDetailsRequest (string "YYYY-MM-DD")
				// It must be parsed to time.Time for the model.
				if req.EventDetails.EventDate != "" { // Check if date string is actually provided
					eventDate, errDate := time.Parse("2006-01-02", req.EventDetails.EventDate)
					if errDate != nil {
						s.logger.Warn("Invalid event_date format during listing update",
							zap.String("listingID", id.String()),
							zap.String("eventDate", req.EventDetails.EventDate),
							zap.Error(errDate))
						// Potentially return common.ErrBadRequest here if date format is crucial and invalid
						// For now, we log and skip updating the date if parsing fails.
						// Or, rely on validator on request struct. If it's here, it means validator passed.
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

<<<<<<< HEAD
=======
	// Status and IsAdminApproved fields are NOT modified by this user-facing update method.
	// Those are handled by admin-specific methods like AdminUpdateListingStatus.
	// If a listing is edited, does it need re-approval?
	// Business rule: For simplicity, assume edits to active listings don't require re-approval,
	// unless it was pending, then it remains pending.
	// If it was rejected, editing might move it to pending again.
>>>>>>> origin/feat/user-listings-management
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
	return updatedListing, nil
}

// DeleteListing handles deleting a listing.
func (s *ServiceImplementation) DeleteListing(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		s.logger.Error("Failed to delete listing", zap.Error(err), zap.String("listingID", id.String()), zap.String("userID", userID.String()))
		return err
	}
	s.logger.Info("Listing deleted successfully", zap.String("listingID", id.String()), zap.String("userID", userID.String()))
	return nil
}

// SearchListings performs a search for listings based on various criteria.
func (s *ServiceImplementation) SearchListings(ctx context.Context, query ListingSearchQuery, authenticatedUserID *uuid.UUID) ([]Listing, *common.Pagination, error) {
	if query.MaxDistanceKM == nil {
		maxDistConfig, err := s.getPlatformConfigInt("MAX_LISTING_DISTANCE_KM")
		if err == nil && maxDistConfig > 0 {
			floatMaxDist := float64(maxDistConfig)
			query.MaxDistanceKM = &floatMaxDist
		} else if err != nil {
			s.logger.Warn("Could not parse MAX_LISTING_DISTANCE_KM, not applying default distance filter", zap.Error(err))
		}
	}

	if query.Latitude != nil && query.Longitude != nil && query.SortBy == "" {
		query.SortBy = "distance"
	}

	listings, pagination, err := s.repo.Search(ctx, query)
	if err != nil {
		s.logger.Error("Failed to search listings", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not retrieve listings.")
	}
	return listings, pagination, nil
}

<<<<<<< HEAD
// AdminUpdateListingStatus handles admin updates to a listing's status.
func (s *ServiceImplementation) AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, newStatus ListingStatus, adminNotes *string) (*Listing, error) {
	listingBeforeUpdate, err := s.repo.FindByID(ctx, id, true)
=======
func (s *service) GetUserListings(ctx context.Context, userID uuid.UUID, query UserListingsQuery) ([]Listing, *common.Pagination, error) {
	listings, pagination, err := s.repo.FindByUserID(ctx, userID, query)
	if err != nil {
		s.logger.Error("Failed to get user listings from repository",
			zap.String("userID", userID.String()),
			zap.Any("query", query), // Be mindful of logging sensitive query params if any
			zap.Error(err),
		)
		// Directly return the error from the repository, which should be one of common.Err types or a generic error
		return nil, nil, err
	}

	s.logger.Debug("Successfully retrieved user listings",
		zap.String("userID", userID.String()),
		zap.Int("count", len(listings)),
	)
	return listings, pagination, nil
}

// --- Admin Methods ---
func (s *service) AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, false) // Don't need full preload just for status update
>>>>>>> origin/feat/user-listings-management
	if err != nil {
		s.logger.Warn("AdminUpdateListingStatus: Listing not found before update", zap.String("listingID", id.String()), zap.Error(err))
		return nil, err
	}
	originalStatus := listingBeforeUpdate.Status
	originalIsAdminApproved := listingBeforeUpdate.IsAdminApproved

	userWasUpdated := false
	if newStatus == StatusActive && originalStatus == StatusPendingApproval && !listingBeforeUpdate.User.IsFirstPostApproved {
		postingUser := listingBeforeUpdate.User
		if !postingUser.IsFirstPostApproved {
			postingUser.IsFirstPostApproved = true
			fullUser, userErr := s.userRepo.FindByID(ctx, postingUser.ID)
			if userErr == nil {
				fullUser.IsFirstPostApproved = true
				if updateErr := s.userRepo.Update(ctx, fullUser); updateErr != nil {
					s.logger.Error("Failed to update user IsFirstPostApproved flag", zap.Error(updateErr), zap.String("userID", postingUser.ID.String()))
				} else {
					s.logger.Info("User's first post approved, flag updated", zap.String("userID", postingUser.ID.String()))
					userWasUpdated = true
				}
			} else {
				s.logger.Error("Failed to find user to update IsFirstPostApproved flag", zap.Error(userErr), zap.String("userID", postingUser.ID.String()))
			}
		}
	}

	if err := s.repo.UpdateStatus(ctx, id, newStatus, adminNotes); err != nil {
		s.logger.Error("Failed to admin update listing status in repo", zap.Error(err), zap.String("listingID", id.String()))
		return nil, err
	}

	if newStatus == StatusActive {
		tempListingForApprovalUpdate, findErr := s.repo.FindByID(ctx, id, false)
		if findErr == nil {
			tempListingForApprovalUpdate.IsAdminApproved = true
			if errUpdate := s.repo.Update(ctx, tempListingForApprovalUpdate); errUpdate != nil {
				s.logger.Error("Failed to explicitly set IsAdminApproved to true after status update", zap.Error(errUpdate), zap.String("listingID", id.String()))
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
