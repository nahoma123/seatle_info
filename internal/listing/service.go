// File: internal/listing/service.go
package listing

import (
	"context"
	"errors"
	"time"

	"seattle_info_backend/internal/category" // For category validation
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/user" // For user details, first post approval status

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

	// Admin specific
	AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) (*Listing, error)
	AdminApproveListing(ctx context.Context, id uuid.UUID) (*Listing, error)
	AdminGetListingByID(ctx context.Context, id uuid.UUID) (*Listing, error) // Bypasses some user checks

	// Jobs related (can be called by cron jobs)
	ExpireListings(ctx context.Context) (int, error)
}

type service struct {
	repo            Repository
	userRepo        user.Repository  // To check user's first post status, etc.
	categoryService category.Service // To validate category/subcategory IDs
	cfg             *config.Config
	logger          *zap.Logger
}

// NewService creates a new listing service.
func NewService(
	repo Repository,
	userRepo user.Repository,
	categoryService category.Service,
	cfg *config.Config,
	logger *zap.Logger,
) Service {
	return &service{
		repo:            repo,
		userRepo:        userRepo,
		categoryService: categoryService,
		cfg:             cfg,
		logger:          logger,
	}
}

func (s *service) CreateListing(ctx context.Context, userID uuid.UUID, req CreateListingRequest) (*Listing, error) {
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
	return createdListing, nil
}

func (s *service) GetListingByID(ctx context.Context, id uuid.UUID, authenticatedUserID *uuid.UUID) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, true) // Preload associations
	if err != nil {
		return nil, err // Repo returns common.ErrNotFound
	}

	// BR3.4: Unregistered users cannot view contact information.
	// This is handled by ToListingResponse, which needs `isAuthenticated`.
	// Here, we check if the listing should even be visible.
	// If listing is PENDING_APPROVAL, only owner or admin should see it.
	if listing.Status == StatusPendingApproval {
		isOwner := authenticatedUserID != nil && listing.UserID == *authenticatedUserID
		// TODO: Add admin role check here. For now, assume if not owner, it's forbidden.
		// For simplicity, if authenticatedUserID is nil, it's an unauth user trying to view pending.
		if !isOwner {
			// Need a way to check if authenticatedUserID is an admin.
			// This requires passing user role or fetching user details based on authenticatedUserID.
			// For now, just restrict to owner.
			s.logger.Warn("Attempt to view pending listing by non-owner/non-admin",
				zap.String("listingID", id.String()),
				zap.Any("viewerID", authenticatedUserID),
			)
			return nil, common.ErrNotFound.WithDetails("Listing not found or access denied.") // Mask as NotFound
		}
	}

	// BR2.3: Listing Expiry. If listing is expired, treat as not found unless specifically requested by admin/owner
	if listing.Status == StatusExpired && (authenticatedUserID == nil || listing.UserID != *authenticatedUserID) {
		// (Add admin check here too if admins should see expired listings)
		return nil, common.ErrNotFound.WithDetails("Listing not found or has expired.")
	}

	return listing, nil
}

func (s *service) AdminGetListingByID(ctx context.Context, id uuid.UUID) (*Listing, error) {
	// Bypasses normal visibility rules, for admin panel use.
	listing, err := s.repo.FindByID(ctx, id, true) // Preload associations
	if err != nil {
		return nil, err
	}
	return listing, nil
}

func (s *service) UpdateListing(ctx context.Context, id uuid.UUID, userID uuid.UUID, req UpdateListingRequest) (*Listing, error) {
	existingListing, err := s.repo.FindByID(ctx, id, true) // Preload for easier updates
	if err != nil {
		return nil, err
	}

	if existingListing.UserID != userID {
		// TODO: Add admin role check if admins can edit any listing
		s.logger.Warn("User attempted to update a listing they do not own",
			zap.String("listingID", id.String()),
			zap.String("editorUserID", userID.String()),
			zap.String("ownerUserID", existingListing.UserID.String()))
		return nil, common.ErrForbidden.WithDetails("You do not have permission to update this listing.")
	}

	// --- Update fields ---
	// Category/SubCategory change logic can be complex (e.g., what happens to details?)
	// For now, disallow changing CategoryID. SubCategoryID can be changed within the same Category.
	if req.CategoryID != nil && *req.CategoryID != existingListing.CategoryID {
		return nil, common.ErrBadRequest.WithDetails("Changing the main category of a listing is not allowed. Please create a new listing.")
	}
	if req.SubCategoryID != nil { // If a new subcategory is provided
		// Validate that the new subcategory belongs to the existing listing's category
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
		// If request explicitly sets subcategory to null (and it wasn't null before)
		// We need to check if the category requires a subcategory (e.g. "Businesses")
		currentCategory, _ := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
		if currentCategory != nil && currentCategory.Slug == "businesses" { // Example slug
			return nil, common.ErrBadRequest.WithDetails("Cannot remove subcategory from a 'Business' listing.")
		}
		existingListing.SubCategoryID = nil // Allow removing subcategory if not required
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
	} else if locationChanged && (existingListing.Latitude == nil || existingListing.Longitude == nil) { // one is nil, other not
		existingListing.Location = nil // Clear location if lat/lon incomplete
		existingListing.Latitude = nil
		existingListing.Longitude = nil
	}

	// Update specific details (this part can get complex)
	// If new details are provided, update them. If not, keep existing or clear if explicitly nulled.
	// This example assumes if a details block is in the request, it replaces existing details for that type.
	currentCat, _ := s.categoryService.GetCategoryByID(ctx, existingListing.CategoryID, false)
	if currentCat != nil {
		switch currentCat.Slug {
		case "baby-sitting":
			if req.BabysittingDetails != nil {
				if existingListing.BabysittingDetails == nil {
					existingListing.BabysittingDetails = &ListingDetailsBabysitting{}
				}
				existingListing.BabysittingDetails.LanguagesSpoken = req.BabysittingDetails.LanguagesSpoken
			}
		case "housing":
			if req.HousingDetails != nil {
				if existingListing.HousingDetails == nil {
					existingListing.HousingDetails = &ListingDetailsHousing{}
				}
				existingListing.HousingDetails.PropertyType = req.HousingDetails.PropertyType
				existingListing.HousingDetails.RentDetails = req.HousingDetails.RentDetails
				existingListing.HousingDetails.SalePrice = req.HousingDetails.SalePrice
			}
		case "events":
			if req.EventDetails != nil {
				if existingListing.EventDetails == nil {
					existingListing.EventDetails = &ListingDetailsEvents{}
				}
				eventDate, _ := time.Parse("2006-01-02", req.EventDetails.EventDate)
				existingListing.EventDetails.EventDate = eventDate
				existingListing.EventDetails.EventTime = req.EventDetails.EventTime
				existingListing.EventDetails.OrganizerName = req.EventDetails.OrganizerName
				existingListing.EventDetails.VenueName = req.EventDetails.VenueName
			}
		}
	}

	// If a listing is edited, does it need re-approval?
	// Business rule: For simplicity, assume edits to active listings don't require re-approval,
	// unless it was pending, then it remains pending.
	// If it was rejected, editing might move it to pending again.
	if existingListing.Status == StatusRejected || existingListing.Status == StatusAdminRemoved {
		// If an admin allows editing rejected/removed posts, they might go back to pending.
		// This depends on business rules not explicitly stated. For now, assume they cannot be edited
		// or if they can, they remain in their current state unless admin changes it.
		// Or, if owner edits a rejected post, it goes back to pending.
		// existingListing.Status = StatusPendingApproval;
		// existingListing.IsAdminApproved = false;
	}

	if err := s.repo.Update(ctx, existingListing); err != nil {
		s.logger.Error("Failed to update listing in repository", zap.Error(err), zap.String("listingID", id.String()))
		return nil, err
	}

	// Reload the updated listing with associations for the response
	updatedListing, err := s.repo.FindByID(ctx, existingListing.ID, true)
	if err != nil {
		s.logger.Error("Failed to reload updated listing with associations", zap.String("listingID", existingListing.ID.String()), zap.Error(err))
		return existingListing, nil // Or return error
	}

	s.logger.Info("Listing updated successfully", zap.String("listingID", updatedListing.ID.String()))
	return updatedListing, nil
}

func (s *service) DeleteListing(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	// Repository Delete already checks ownership.
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		s.logger.Error("Failed to delete listing", zap.Error(err), zap.String("listingID", id.String()), zap.String("userID", userID.String()))
		return err
	}
	s.logger.Info("Listing deleted successfully", zap.String("listingID", id.String()), zap.String("userID", userID.String()))
	return nil
}

func (s *service) SearchListings(ctx context.Context, query ListingSearchQuery, authenticatedUserID *uuid.UUID) ([]Listing, *common.Pagination, error) {
	// BR2.2: Max distance threshold from admin config
	if query.MaxDistanceKM == nil { // If user didn't specify, use admin default
		maxDistConfig, err := s.getPlatformConfigInt("MAX_LISTING_DISTANCE_KM")
		if err == nil && maxDistConfig > 0 {
			floatMaxDist := float64(maxDistConfig)
			query.MaxDistanceKM = &floatMaxDist
		} else if err != nil {
			s.logger.Warn("Could not parse MAX_LISTING_DISTANCE_KM, not applying default distance filter", zap.Error(err))
		}
	}

	// BR2.1: Default sort by proximity (nearest first), secondary by recency.
	// The repository handles ST_Distance for sorting if lat/lon and sort_by=distance are provided.
	// If lat/lon provided but no sort_by, we should default to distance sort.
	if query.Latitude != nil && query.Longitude != nil && query.SortBy == "" {
		query.SortBy = "distance" // Default to distance sort if location is given
	}
	// If no location, default to recency (created_at DESC) - repo handles this.

	listings, pagination, err := s.repo.Search(ctx, query)
	if err != nil {
		s.logger.Error("Failed to search listings", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not retrieve listings.")
	}
	return listings, pagination, nil
}

// --- Admin Methods ---
func (s *service) AdminUpdateListingStatus(ctx context.Context, id uuid.UUID, status ListingStatus, adminNotes *string) (*Listing, error) {
	listing, err := s.repo.FindByID(ctx, id, false) // Don't need full preload just for status update
	if err != nil {
		return nil, err
	}

	// If approving a previously pending post, update user's IsFirstPostApproved status
	if status == StatusActive && listing.Status == StatusPendingApproval && !listing.User.IsFirstPostApproved {
		// This requires fetching the User object to update IsFirstPostApproved.
		// This is simplified. In a real scenario, you'd fetch the User object via userRepo,
		// update it, and save it. Here, we assume this side effect happens.
		// This is a good candidate for an event-driven approach or a dedicated user service call.
		// For now, just update the listing's IsAdminApproved flag.
		listing.IsAdminApproved = true
		// And also update the user record
		if postingUser, userErr := s.userRepo.FindByID(ctx, listing.UserID); userErr == nil {
			if !postingUser.IsFirstPostApproved {
				postingUser.IsFirstPostApproved = true
				if updateErr := s.userRepo.Update(ctx, postingUser); updateErr != nil {
					s.logger.Error("Failed to update user IsFirstPostApproved flag", zap.Error(updateErr), zap.String("userID", postingUser.ID.String()))
					// Continue, but log the error.
				} else {
					s.logger.Info("User's first post approved, flag updated", zap.String("userID", postingUser.ID.String()))
				}
			}
		} else {
			s.logger.Error("Failed to find user to update IsFirstPostApproved flag", zap.Error(userErr), zap.String("userID", listing.UserID.String()))
		}
	}

	listing.Status = status
	// `adminNotes` would typically be stored in a separate audit log or a field on the listing if it exists.
	// For now, we pass it to repo if it needs to do something with it.

	if err := s.repo.UpdateStatus(ctx, id, status, adminNotes); err != nil { // repo.UpdateStatus just updates the status field
		s.logger.Error("Failed to admin update listing status in repo", zap.Error(err), zap.String("listingID", id.String()))
		return nil, err
	}

	// Reload to get current state
	updatedListing, err := s.repo.FindByID(ctx, id, true)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Admin updated listing status", zap.String("listingID", id.String()), zap.String("newStatus", string(status)))
	return updatedListing, nil
}

func (s *service) AdminApproveListing(ctx context.Context, id uuid.UUID) (*Listing, error) {
	return s.AdminUpdateListingStatus(ctx, id, StatusActive, nil)
}

// --- Job Methods ---
func (s *service) ExpireListings(ctx context.Context) (int, error) {
	now := time.Now()
	expiredListings, err := s.repo.FindExpiredListings(ctx, now)
	if err != nil {
		s.logger.Error("Failed to find expired listings", zap.Error(err))
		return 0, err
	}

	count := 0
	for _, listing := range expiredListings {
		listing.Status = StatusExpired
		// Use UpdateStatus which is simpler, or repo.Update if more fields change
		if err := s.repo.UpdateStatus(ctx, listing.ID, StatusExpired, nil); err != nil {
			s.logger.Error("Failed to update listing to expired", zap.Error(err), zap.String("listingID", listing.ID.String()))
			// Continue to next listing
		} else {
			s.logger.Info("Listing expired and status updated", zap.String("listingID", listing.ID.String()))
			count++
		}
	}
	s.logger.Info("Listing expiry job completed", zap.Int("expired_count", count), zap.Int("found_to_expire", len(expiredListings)))
	return count, nil
}

// Helper to get config values from app_configurations table (conceptual)
// This would ideally use a dedicated config repository/service.
// For now, it's a placeholder for how one might fetch dynamic configs.
// These are used for BR2.2, BR2.3, BR3.3
func (s *service) getPlatformConfigDate(key string) (*time.Time, error) {
	// Placeholder: In a real app, fetch this from a AppConfigurationService/Repository
	// For example, value could be "2024-12-31"
	if key == "FIRST_POST_APPROVAL_MODEL_ACTIVE_UNTIL" {
		activeMonths := s.cfg.FirstPostApprovalActiveMonths // From .env
		if activeMonths > 0 {
			// This is a static calculation based on server start or a fixed date.
			// A dynamic DB value would be better.
			// For this example, let's assume it's N months from a fixed project launch date.
			// Or, for simplicity, N months from now (less ideal for a fixed period).
			// For this example, let's use the .env config value as the duration from *now*
			// which isn't how it *should* work for a fixed platform launch period, but simplifies.
			// A real implementation would parse a date string from the DB.
			val := time.Now().AddDate(0, activeMonths, 0)
			return &val, nil
		}
	}
	return nil, errors.New("config key not found or not a date: " + key)
}

func (s *service) getPlatformConfigInt(key string) (int, error) {
	// Placeholder:
	if key == "DEFAULT_LISTING_LIFESPAN_DAYS" {
		return s.cfg.DefaultListingLifespanDays, nil // From .env
	}
	if key == "MAX_LISTING_DISTANCE_KM" {
		return s.cfg.MaxListingDistanceKM, nil // From .env
	}
	return 0, errors.New("config key not found or not an int: " + key)
}
