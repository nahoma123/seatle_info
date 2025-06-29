// File: internal/listing/model.go
package listing

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"

	"seattle_info_backend/internal/category" // For Category and SubCategory response in Listing
	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/user" // For User response in Listing

	"github.com/google/uuid"
	"github.com/lib/pq" // For pq.StringArray
)

// PostGISPoint represents a geographical point for PostGIS.
type PostGISPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Value implements the driver.Valuer interface for PostGISPoint.
func (p PostGISPoint) Value() (driver.Value, error) {
	if p.Lat == 0 && p.Lon == 0 {
		return nil, nil
	}
	return fmt.Sprintf("SRID=4326;POINT(%f %f)", p.Lon, p.Lat), nil
}

// Scan implements the sql.Scanner interface for PostGISPoint.
func (p *PostGISPoint) Scan(value interface{}) error {
	if value == nil {
		p.Lat, p.Lon = 0, 0
		return nil
	}
	s, ok := value.(string)
	if !ok {
		if b, okByte := value.([]byte); okByte {
			s = string(b)
		} else {
			return errors.New("failed to scan PostGISPoint: invalid type")
		}
	}
	s = strings.TrimPrefix(s, "SRID=4326;")
	s = strings.TrimPrefix(s, "POINT(")
	s = strings.TrimSuffix(s, ")")
	parts := strings.Fields(s)
	if len(parts) == 2 {
		if _, err := fmt.Sscanf(parts[0], "%f", &p.Lon); err != nil {
			return fmt.Errorf("failed to parse longitude: %w", err)
		}
		if _, err := fmt.Sscanf(parts[1], "%f", &p.Lat); err != nil {
			return fmt.Errorf("failed to parse latitude: %w", err)
		}
		return nil
	}
	return errors.New("failed to scan PostGISPoint: invalid format, expected POINT(lon lat)")
}

// --- Main Listing Model ---
type ListingStatus string

const (
	StatusPendingApproval ListingStatus = "pending_approval"
	StatusActive          ListingStatus = "active"
	StatusExpired         ListingStatus = "expired"
	StatusRejected        ListingStatus = "rejected"
	StatusAdminRemoved    ListingStatus = "admin_removed"
)

type Listing struct {
	common.BaseModel
	UserID        uuid.UUID             `gorm:"type:uuid;not null"`
	User          *user.User            `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CategoryID    uuid.UUID             `gorm:"type:uuid;not null"`
	Category      category.Category     `gorm:"foreignKey:CategoryID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	SubCategoryID *uuid.UUID            `gorm:"type:uuid"`
	SubCategory   *category.SubCategory `gorm:"foreignKey:SubCategoryID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Title         string                `gorm:"type:varchar(255);not null"`
	Description   string                `gorm:"type:text;not null"`
	Status        ListingStatus         `gorm:"type:varchar(50);not null;default:'active'"`
	ContactName   *string               `gorm:"type:varchar(150)"`
	ContactEmail  *string               `gorm:"type:varchar(255)"`
	ContactPhone  *string               `gorm:"type:varchar(50)"`
	AddressLine1  *string               `gorm:"type:varchar(255)"`
	AddressLine2  *string               `gorm:"type:varchar(255)"`
	City          *string               `gorm:"type:varchar(100);default:'Seattle'"`
	State         *string               `gorm:"type:varchar(50);default:'WA'"`
	ZipCode       *string               `gorm:"type:varchar(20)"`
	Latitude      *float64              `gorm:"type:decimal(10,8)"`
	Longitude     *float64              `gorm:"type:decimal(11,8)"`
	Location      *PostGISPoint         `gorm:"-"`
	LocationWKT   string                `gorm:"column:location_wkt;->:false"`

	ExpiresAt          time.Time                  `gorm:"not null"`
	IsAdminApproved    bool                       `gorm:"not null;default:false"`
	BabysittingDetails *ListingDetailsBabysitting `gorm:"foreignKey:ListingID;references:ID;constraint:OnDelete:CASCADE;"`
	HousingDetails     *ListingDetailsHousing     `gorm:"foreignKey:ListingID;references:ID;constraint:OnDelete:CASCADE;"`
	EventDetails       *ListingDetailsEvents      `gorm:"foreignKey:ListingID;references:ID;constraint:OnDelete:CASCADE;"`
	Images             []ListingImage             `gorm:"foreignKey:ListingID;constraint:OnDelete:CASCADE;"`
}

func (Listing) TableName() string {
	return "listings"
}

// --- Listing Image Model ---
type ListingImage struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ListingID uuid.UUID `json:"listing_id" gorm:"type:uuid;not null"`
	ImagePath string    `json:"-" gorm:"type:text;not null"`      // Relative path within IMAGE_STORAGE_PATH, not directly exposed
	ImageURL  string    `json:"image_url" gorm:"-"`               // Dynamically generated, not stored in DB
	SortOrder int       `json:"sort_order" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"` // For GORM to auto-update
}

func (ListingImage) TableName() string {
	return "listing_images"
}

// PopulateImageURL generates the full URL for an image.
// It needs the base URL from config. This function would typically be called
// in the service layer or when transforming the model to a response DTO.
func (li *ListingImage) PopulateImageURL(baseURL string) {
	if li.ImagePath != "" {
		// Ensure no double slashes if baseURL already ends with / and ImagePath starts with /
		li.ImageURL = strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(li.ImagePath, "/")
	}
}

// --- Listing Detail Models ---
type ListingDetailsBabysitting struct {
	ListingID       uuid.UUID      `gorm:"type:uuid;primaryKey"`
	LanguagesSpoken pq.StringArray `gorm:"type:text[]"`
}

func (ListingDetailsBabysitting) TableName() string {
	return "listing_details_babysitting"
}

type HousingPropertyType string

const (
	HousingForRent HousingPropertyType = "for_rent"
	HousingForSale HousingPropertyType = "for_sale"
)

type ListingDetailsHousing struct {
	ListingID    uuid.UUID           `gorm:"type:uuid;primaryKey"`
	PropertyType HousingPropertyType `gorm:"type:varchar(50);not null"`
	RentDetails  *string             `gorm:"type:varchar(255)"`
	SalePrice    *float64            `gorm:"type:numeric(12,2)"`
}

func (ListingDetailsHousing) TableName() string {
	return "listing_details_housing"
}

type ListingDetailsEvents struct {
	ListingID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	EventDate     time.Time `gorm:"type:date;not null"`
	EventTime     *string   `gorm:"type:time"`
	OrganizerName *string   `gorm:"type:varchar(150)"`
	VenueName     *string   `gorm:"type:varchar(255)"`
}

func (ListingDetailsEvents) TableName() string {
	return "listing_details_events"
}

// --- DTOs for API ---
type CreateListingBabysittingDetailsRequest struct {
	LanguagesSpoken []string `json:"languages_spoken" binding:"omitempty,dive,max=50"`
}

type CreateListingHousingDetailsRequest struct {
	PropertyType HousingPropertyType `json:"property_type" binding:"required,oneof=for_rent for_sale"`
	RentDetails  *string             `json:"rent_details,omitempty" binding:"omitempty,max=255"`
	SalePrice    *float64            `json:"sale_price,omitempty" binding:"omitempty,gt=0"`
}

type CreateListingEventDetailsRequest struct {
	EventDate     string  `json:"event_date" binding:"required,datetime=2006-01-02"`
	EventTime     *string `json:"event_time,omitempty" binding:"omitempty,datetime=15:04:05"`
	OrganizerName *string `json:"organizer_name,omitempty" binding:"omitempty,max=150"`
	VenueName     *string `json:"venue_name,omitempty" binding:"omitempty,max=255"`
}

type CreateListingRequest struct {
	CategoryID         uuid.UUID                               `json:"category_id" binding:"required"`
	SubCategoryID      *uuid.UUID                              `json:"sub_category_id,omitempty"`
	Title              string                                  `json:"title" binding:"required,min=5,max=255"`
	Description        string                                  `json:"description" binding:"required,min=20"`
	ContactName        *string                                 `json:"contact_name,omitempty" binding:"omitempty,max=150"`
	ContactEmail       *string                                 `json:"contact_email,omitempty" binding:"omitempty,email,max=255"`
	ContactPhone       *string                                 `json:"contact_phone,omitempty" binding:"omitempty,max=50"`
	AddressLine1       *string                                 `json:"address_line1,omitempty" binding:"omitempty,max=255"`
	AddressLine2       *string                                 `json:"address_line2,omitempty" binding:"omitempty,max=255"`
	City               *string                                 `json:"city,omitempty" binding:"omitempty,max=100"`
	State              *string                                 `json:"state,omitempty" binding:"omitempty,max=50"`
	ZipCode            *string                                 `json:"zip_code,omitempty" binding:"omitempty,max=20"`
	Latitude           *float64                                `json:"latitude,omitempty" binding:"omitempty,latitude"`
	Longitude          *float64                                `json:"longitude,omitempty" binding:"omitempty,longitude"`
	BabysittingDetails *CreateListingBabysittingDetailsRequest `json:"babysitting_details,omitempty"`
	HousingDetails     *CreateListingHousingDetailsRequest     `json:"housing_details,omitempty"`
	EventDetails       *CreateListingEventDetailsRequest       `json:"event_details,omitempty"`
	// Images are handled via multipart/form-data in the handler, not directly in this struct for JSON binding.
	// The handler will need to manually process c.Request.MultipartForm.File["images"]
}

type UpdateListingRequest struct {
	CategoryID         *uuid.UUID                              `json:"category_id,omitempty"`
	SubCategoryID      *uuid.UUID                              `json:"sub_category_id,omitempty"`
	Title              *string                                 `json:"title,omitempty" binding:"omitempty,min=5,max=255"`
	Description        *string                                 `json:"description,omitempty" binding:"omitempty,min=20"`
	ContactName        *string                                 `json:"contact_name,omitempty" binding:"omitempty,max=150"`
	ContactEmail       *string                                 `json:"contact_email,omitempty" binding:"omitempty,email,max=255"`
	ContactPhone       *string                                 `json:"contact_phone,omitempty" binding:"omitempty,max=50"`
	AddressLine1       *string                                 `json:"address_line1,omitempty" binding:"omitempty,max=255"`
	AddressLine2       *string                                 `json:"address_line2,omitempty" binding:"omitempty,max=255"`
	City               *string                                 `json:"city,omitempty" binding:"omitempty,max=100"`
	State              *string                                 `json:"state,omitempty" binding:"omitempty,max=50"`
	ZipCode            *string                                 `json:"zip_code,omitempty" binding:"omitempty,max=20"`
	Latitude           *float64                                `json:"latitude,omitempty" binding:"omitempty,latitude"`
	Longitude          *float64                                `json:"longitude,omitempty" binding:"omitempty,longitude"`
	BabysittingDetails *CreateListingBabysittingDetailsRequest `json:"babysitting_details,omitempty"`
	HousingDetails     *CreateListingHousingDetailsRequest     `json:"housing_details,omitempty"`
	EventDetails       *CreateListingEventDetailsRequest       `json:"event_details,omitempty"`
	// Images are handled via multipart/form-data in the handler for new uploads.
	// Existing images to remove might be specified by their IDs.
	RemoveImageIDs []uuid.UUID `json:"remove_image_ids,omitempty"`
}

type ListingImageResponse struct {
	ID       uuid.UUID `json:"id"`
	ImageURL string    `json:"image_url"`
	SortOrder int       `json:"sort_order"`
}

type ListingResponse struct {
	ID                 uuid.UUID                     `json:"id"`
	UserID             uuid.UUID                     `json:"user_id"`
	User               user.UserResponse             `json:"user"`
	CategoryID         uuid.UUID                     `json:"category_id"`
	Category           category.CategoryResponse     `json:"category"`
	SubCategory        *category.SubCategoryResponse `json:"sub_category,omitempty"`
	Title              string                        `json:"title"`
	Description        string                        `json:"description"`
	Status             ListingStatus                 `json:"status"`
	ContactName        *string                       `json:"contact_name,omitempty"`
	ContactEmail       *string                       `json:"contact_email,omitempty"`
	ContactPhone       *string                       `json:"contact_phone,omitempty"`
	AddressLine1       *string                       `json:"address_line1,omitempty"`
	AddressLine2       *string                       `json:"address_line2,omitempty"`
	City               *string                       `json:"city,omitempty"`
	State              *string                       `json:"state,omitempty"`
	ZipCode            *string                       `json:"zip_code,omitempty"`
	Latitude           *float64                      `json:"latitude,omitempty"`
	Longitude          *float64                      `json:"longitude,omitempty"`
	Location           *PostGISPoint                 `json:"location,omitempty"`
	Distance           *float64                      `json:"distance_km,omitempty"`
	ExpiresAt          time.Time                     `json:"expires_at"`
	IsAdminApproved    bool                          `json:"is_admin_approved"`
	CreatedAt          time.Time                     `json:"created_at"`
	UpdatedAt          time.Time                     `json:"updated_at"`
	BabysittingDetails *ListingDetailsBabysitting    `json:"babysitting_details,omitempty"`
	HousingDetails     *ListingDetailsHousing        `json:"housing_details,omitempty"`
	EventDetails       *ListingDetailsEvents         `json:"event_details,omitempty"`
	Images             []ListingImageResponse        `json:"images,omitempty"`
}

func ToListingResponse(listing *Listing, isAuthenticated bool, imageBaseURL string) ListingResponse {
	sharedUser := user.DBToShared(listing.User) // Convert GORM user.User to shared.User
	userResp := user.ToUserResponse(sharedUser) // Pass shared.User to ToUserResponse
	catResp := category.ToCategoryResponse(&listing.Category)
	var subCatResp *category.SubCategoryResponse
	if listing.SubCategory != nil {
		tempSubCatResp := category.ToSubCategoryResponse(listing.SubCategory)
		subCatResp = &tempSubCatResp
	}

	resp := ListingResponse{
		ID:                 listing.ID,
		UserID:             listing.UserID,
		User:               userResp,
		CategoryID:         listing.CategoryID,
		Category:           catResp,
		SubCategory:        subCatResp,
		Title:              listing.Title,
		Description:        listing.Description,
		Status:             listing.Status,
		ContactName:        listing.ContactName,
		AddressLine1:       listing.AddressLine1,
		AddressLine2:       listing.AddressLine2,
		City:               listing.City,
		State:              listing.State,
		ZipCode:            listing.ZipCode,
		Latitude:           listing.Latitude,
		Longitude:          listing.Longitude,
		Location:           listing.Location,
		ExpiresAt:          listing.ExpiresAt,
		IsAdminApproved:    listing.IsAdminApproved,
		CreatedAt:          listing.CreatedAt,
		UpdatedAt:          listing.UpdatedAt,
		BabysittingDetails: listing.BabysittingDetails,
		HousingDetails:     listing.HousingDetails,
		EventDetails:       listing.EventDetails,
		// Images will be populated below
	}

	if len(listing.Images) > 0 {
		resp.Images = make([]ListingImageResponse, len(listing.Images))
		for i, img := range listing.Images {
			img.PopulateImageURL(imageBaseURL) // Use the PopulateImageURL method
			resp.Images[i] = ListingImageResponse{
				ID:        img.ID,
				ImageURL:  img.ImageURL,
				SortOrder: img.SortOrder,
			}
		}
	}

	if isAuthenticated {
		resp.ContactEmail = listing.ContactEmail
		resp.ContactPhone = listing.ContactPhone
	}
	return resp
}

type AdminUpdateListingStatusRequest struct {
	Status     ListingStatus `json:"status" binding:"required,oneof=pending_approval active expired rejected admin_removed"`
	AdminNotes *string       `json:"admin_notes,omitempty"`
}

type ListingSearchQuery struct {
	common.PaginationQuery
	SearchTerm     string   `form:"q"`
	CategoryID     *string  `form:"category_id"`
	SubCategoryID  *string  `form:"sub_category_id"`
	UserID         *string  `form:"user_id"`
	Status         string   `form:"status"`
	Latitude       *float64 `form:"lat"`
	Longitude      *float64 `form:"lon"`
	MaxDistanceKM  *float64 `form:"max_distance_km"`
	SortBy         string   `form:"sort_by"`
	SortOrder      string   `form:"sort_order"`
	IncludeExpired bool     `form:"include_expired"`
}

type UserListingsQuery struct {
	common.PaginationQuery
	Status       *string `form:"status"`
	CategorySlug *string `form:"category_slug"`
}
